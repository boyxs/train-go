// Package incr 是增量同步引擎。
//
// 设计（architecture.md §8.3 + §14 任务 #4）：
//
//	Run    从 checkpoint 续订 Source.IncrSubscribe → 按 PK hash 分 N partition → 每 partition 独立攒批 → Sink.Apply → 持久化 binlog 位点；
//	Pause  cancel 注册的 ctx；
//	Lag    通过 source.LagReporter 接口拿任务延迟（毫秒）。
//
// 多表动态调度：构造时持 SourceFactory / SinkFactory；Run 入口按 taskId lookup task →
// factory.BuildSrc / BuildDst 构造本次跑的 Source/Sink。Source 也存进 sync.Map 让 Lag 能复用。
//
// Partition 并行：
//   - PartitionCount = 1 时退化为单 worker 串行（默认行为）
//   - 多 partition 时，同一 PK 始终落同一 partition（FNV(PK) % N），保证单行变更顺序
//   - 每 partition 独立 checkpoint（shard_no = partition_no）
//   - subscriber / dispatcher / worker 全部进 errgroup，任一失败 → gctx cancel → 全部协调退出（无 goroutine leak）
//
// Resume 正确性（多 partition crash 恢复）：
//   - 各 partition 独立 flush + 更新 ckpt，可能某 partition 已写到 pos=100、另一 partition 还停在 pos=50
//   - IncrSubscribe 用 `min(所有 partition ckpt)` 作起点重订阅
//     →  fast partition（如 P0=100）会收到 [50, 100] 重放事件；Sink 幂等（INSERT ... ON DUPLICATE KEY UPDATE +
//     Version 乐观锁）保证重放安全
//     →  slow partition（如 P5=50）的未 flush 事件不会丢
//   - 防 ckpt 回退：worker 保留 startPos（启动时本 partition 的 ckpt 位点）；flush 时只在新位点 > startPos 才写 DB，
//     避免 fast partition 的 ckpt 被重订阅事件覆盖到小值
package incr

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/boyxs/train-go/webook/migrator/consts"
	"github.com/boyxs/train-go/webook/migrator/domain"
	migratorerrs "github.com/boyxs/train-go/webook/migrator/errs"
	"github.com/boyxs/train-go/webook/migrator/pipeline/sink"
	"github.com/boyxs/train-go/webook/migrator/pipeline/source"
	"github.com/boyxs/train-go/webook/migrator/pipeline/transform"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/service"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	// DefaultBatchSize 增量批量较小，更倾向"低延迟"而非"高吞吐"（与全量相反）。
	DefaultBatchSize  = 100
	DefaultChannelBuf = 4096
	// DefaultPartitionCount 默认单 partition；按 PK hash 分 N partition 启用并行需 Config 显式配置。
	DefaultPartitionCount = 1
	// DefaultFlushInterval 时间维度强制 flush 间隔。cdc 实时场景批量稀疏（每秒几条事件），
	// 没这个机制会卡在 batch 攒批等满 BatchSize → dst 永不更新。
	// 1s 是延迟和吞吐的权衡:更短 → 更实时 Sink 调用更频繁;更长 → 更批量但延迟高
	DefaultFlushInterval = time.Second
)

// IncrEngine 增量同步引擎接口。
type IncrEngine interface {
	// Run 启动指定 task 的增量同步；从 checkpoint 续订位点，持续消费 binlog 事件。
	// 直到 ctx cancel / Pause / Source.IncrSubscribe 返回 err。
	// 如果同 taskId 已在运行，直接返回 migratorerrs.ErrTaskAlreadyRunning（防并发双开）。
	Run(ctx context.Context, taskId int64) error

	// Pause 取消 Run 中的 task；非阻塞。
	Pause(taskId int64) error

	// IsRunning 判断 taskId 当前是否有 Run 正在执行。handler 层早期判 409 用。
	IsRunning(taskId int64) bool

	// Lag 返回任务最近延迟（毫秒）；Source 不实现 LagReporter → error。
	// 语义：源端 binlog 最新事件时间 → 现在的延迟（snapshot of "我消费到哪了"）。
	Lag(taskId int64) (int64, error)

	// LagDst 返回目标端写入延迟（毫秒）：min 最后一次 Sink.Apply 成功的事件 EventTs → 现在。
	// 反映"已经写到对端最新的数据有多新"。任务未跑 → error；尚无写入返 -1。
	LagDst(taskId int64) (int64, error)

	// RunningTasks 当前运行中的 taskId 列表；监控 Collector 枚举后逐个取 Lag/LagDst。
	RunningTasks() []int64
}

// Config IncrEngine 行为参数。
type Config struct {
	BatchSize      int
	ChannelBuf     int
	PartitionCount int           // 并行 partition 数；<=0 用 DefaultPartitionCount (1)
	FlushInterval  time.Duration // 时间维度 flush 间隔;<=0 用 DefaultFlushInterval (1s)
}

type InternalIncrEngine struct {
	taskSvc      service.TaskService
	ckptRepo     repository.CheckpointRepository
	srcFactory   source.SourceFactory
	sinkFactory  sink.SinkFactory
	transformReg *transform.Registry
	l            logger.LoggerX
	cfg          Config

	paused         sync.Map // map[int64]context.CancelFunc
	runningSources sync.Map // map[int64]source.Source — Lag 复用
	dstLastEventTs sync.Map // map[int64]int64 — task → 最近 Sink.Apply 成功的 EventTs（用于 LagDst）
}

func NewIncrEngine(
	taskSvc service.TaskService,
	ckptRepo repository.CheckpointRepository,
	srcFactory source.SourceFactory,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
	transformReg *transform.Registry,
	cfg Config,
) IncrEngine {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.ChannelBuf <= 0 {
		cfg.ChannelBuf = DefaultChannelBuf
	}
	if cfg.PartitionCount <= 0 {
		cfg.PartitionCount = DefaultPartitionCount
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultFlushInterval
	}
	return &InternalIncrEngine{
		taskSvc: taskSvc, ckptRepo: ckptRepo,
		srcFactory: srcFactory, sinkFactory: sinkFactory,
		transformReg: transformReg,
		l:            l, cfg: cfg,
	}
}

func (e *InternalIncrEngine) Run(ctx context.Context, taskId int64) error {
	// 占位防 race：LoadOrStore 原子操作，已在跑直接返 ErrTaskAlreadyRunning。
	if _, loaded := e.paused.LoadOrStore(taskId, context.CancelFunc(nil)); loaded {
		return migratorerrs.ErrTaskAlreadyRunning
	}
	defer e.paused.Delete(taskId)

	// 标记 IncrRunning；status 失败仅 Warn。
	if err := e.taskSvc.UpdateStatus(ctx, taskId, domain.TaskStatusIncrRunning); err != nil {
		e.l.Warn(ctx, "update task status to incr_running failed",
			logger.Int64("task_id", taskId), logger.Error(err))
	}
	// Incr 通常长跑，正常退出（Pause / ctx cancel）保持 IncrRunning；只有真错误才标 Failed。
	var runErr error
	defer func() {
		if runErr == nil {
			return // 保持 IncrRunning（Pause 或 ctx cancel 也走这里，仍是 IncrRunning，等下次 Run 再激活）
		}
		if err := e.taskSvc.UpdateStatus(context.Background(), taskId, domain.TaskStatusFailed); err != nil {
			e.l.Warn(ctx, "update task status to failed",
				logger.Int64("task_id", taskId), logger.Error(err))
		}
	}()

	task, err := e.taskSvc.Get(ctx, taskId)
	if err != nil {
		runErr = fmt.Errorf("find task %d: %w", taskId, err)
		return runErr
	}
	tables, err := task.Tables()
	if err != nil {
		runErr = err
		return runErr
	}

	runCtx, cancel := context.WithCancel(ctx)
	e.paused.Store(taskId, cancel)

	g, gctx := errgroup.WithContext(runCtx)
	var totalApplied int64
	for tableIdx := range tables {
		ti := tableIdx
		src, err := e.srcFactory.BuildIncrSrc(runCtx, task, ti)
		if err != nil {
			runErr = fmt.Errorf("build src for task %d table %d: %w", taskId, ti, err)
			return runErr
		}
		snk, err := e.sinkFactory.BuildDst(runCtx, task, ti)
		if err != nil {
			runErr = fmt.Errorf("build dst sink for task %d table %d: %w", taskId, ti, err)
			return runErr
		}
		tf, err := e.transformReg.Get(tables[ti].Transform)
		if err != nil {
			runErr = fmt.Errorf("resolve transform for task %d table %d: %w", taskId, ti, err)
			return runErr
		}

		// 把表级 Source 存进 runningSources（Lag 复用）；key 编码 (taskId, tableIdx)
		e.runningSources.Store(runningSourceKey(taskId, ti), src)
		// 注意：tableIdx 闭包捕获 defer 不能用（defer 注册时机问题），延后到 g.Go 完成清理
		g.Go(func() error {
			defer e.runningSources.Delete(runningSourceKey(taskId, ti))
			applied, runErr := e.runTable(gctx, taskId, ti, src, snk, tf)
			atomic.AddInt64(&totalApplied, applied)
			return runErr
		})
	}

	if werr := g.Wait(); werr != nil {
		runErr = werr
		return runErr
	}
	e.l.Info(ctx, "incr engine run done",
		logger.Int64("task_id", taskId),
		logger.Int("tables", len(tables)),
		logger.Int64("applied", totalApplied))
	return nil
}

// runTable 单张表的多 partition 并行流水线：load 各 partition ckpt → min 起订阅 → dispatcher 分发 → N worker。
func (e *InternalIncrEngine) runTable(
	ctx context.Context, taskId int64, tableIdx int, src source.IncrSource, snk sink.Sink, tf transform.Transformer,
) (int64, error) {
	n := e.cfg.PartitionCount

	partCkpts, err := e.loadAllPartitionCheckpoints(ctx, taskId, tableIdx, n)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}
	subCkpt := minPartitionCkpt(partCkpts)

	source2disp := make(chan source.ChangeEvent, e.cfg.ChannelBuf)
	partChans := make([]chan source.ChangeEvent, n)
	for i := 0; i < n; i++ {
		partChans[i] = make(chan source.ChangeEvent, e.cfg.ChannelBuf/n+1)
	}

	g, gctx := errgroup.WithContext(ctx)

	// subscriber
	g.Go(func() error {
		defer close(source2disp)
		subErr := src.IncrSubscribe(gctx, subCkpt, source2disp)
		if subErr != nil && !errors.Is(subErr, context.Canceled) && !errors.Is(subErr, context.DeadlineExceeded) {
			return fmt.Errorf("incr subscribe table %d: %w", tableIdx, subErr)
		}
		return nil
	})

	// dispatcher：用 gctx，worker 失败 → gctx cancel → 退出避免 partChans 阻塞
	g.Go(func() error {
		defer func() {
			for _, ch := range partChans {
				close(ch)
			}
		}()
		for change := range source2disp {
			idx := partitionOf(change.PK, n)
			select {
			case <-gctx.Done():
				return nil
			case partChans[idx] <- change:
			}
		}
		return nil
	})

	// N workers
	var tableApplied int64
	for i := 0; i < n; i++ {
		idx := i
		ch := partChans[i]
		startPos := partCkpts[i].CursorValue
		g.Go(func() error {
			applied, werr := e.runPartition(gctx, taskId, tableIdx, idx, startPos, ch, snk, tf)
			atomic.AddInt64(&tableApplied, applied)
			return werr
		})
	}

	if werr := g.Wait(); werr != nil {
		return tableApplied, werr
	}
	return tableApplied, nil
}

// runningSourceKey 编码 (taskId, tableIdx) 作 runningSources map key。
// taskId 占高位，tableIdx 占低 16 位（最多 65535 张表）。
func runningSourceKey(taskId int64, tableIdx int) int64 {
	return taskId<<16 | int64(tableIdx&0xFFFF)
}

// runPartition 单 (table, partition) 的攒批 / Sink.Apply / checkpoint 流水线。
// startPos 用于 ckpt 防回退；snk 由 Run 闭包传入（多 task 并发安全）。
func (e *InternalIncrEngine) runPartition(
	ctx context.Context, taskId int64, tableIdx, partition int, startPos string,
	ch <-chan source.ChangeEvent, snk sink.Sink, tf transform.Transformer,
) (int64, error) {
	batch := make([]sink.Mutation, 0, e.cfg.BatchSize)
	var lastPos string
	var applied int64

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := snk.Apply(ctx, batch); err != nil {
			return fmt.Errorf("partition %d sink apply: %w", partition, err)
		}
		applied += int64(len(batch))
		// 记录本批 max EventTs → dstLastEventTs（LagDst 用）。
		// 多 partition 并发：用 CAS 保只取 max；前一个值更大时不覆盖。
		var maxEventTs int64
		for _, m := range batch {
			if m.Version > maxEventTs {
				maxEventTs = m.Version
			}
		}
		if maxEventTs > 0 {
			e.updateDstLastEventTs(taskId, maxEventTs)
		}
		// 仅当新位点严格大于 startPos（本 partition 启动时的位点）时才写 ckpt。
		// 等于 startPos 也跳过：等于意味着重放到原点，写一次没意义；小于必然是重放更早事件，写了反而回退。
		if lastPos != "" && compareBinlogPos(lastPos, startPos) > 0 {
			if err := e.updateCheckpointForPartition(ctx, taskId, tableIdx, partition, lastPos); err != nil {
				return fmt.Errorf("table %d partition %d update checkpoint: %w", tableIdx, partition, err)
			}
		}
		batch = batch[:0]
		return nil
	}

	// 时间维度 flush:即使 batch 没满 BatchSize,每 FlushInterval 也强制 flush 一次。
	// 没这个机制时,cdc 实时增量场景(每秒几条事件)会卡死在 batch 攒批等满 100 条,
	// 导致 dst 同步延迟无限增长 + checkpoint 永不推进。
	flushTicker := time.NewTicker(e.cfg.FlushInterval)
	defer flushTicker.Stop()

	// ctx cancel 时 upstream IncrSubscribe 会 close(source2disp),
	// dispatcher 跟着 close(partChans),worker case ok=false 退出 — 不需要 case <-ctx.Done() 显式拦
	for {
		select {
		case change, ok := <-ch:
			if !ok {
				if err := flush(); err != nil {
					return applied, err
				}
				return applied, nil
			}
			m, terr := tf.Transform(changeToMutation(change))
			if terr != nil {
				return applied, fmt.Errorf("transform task %d table %d partition %d pk %q: %w", taskId, tableIdx, partition, change.PK, terr)
			}
			batch = append(batch, m)
			if change.BinlogPos != "" {
				lastPos = change.BinlogPos
			}
			if len(batch) >= e.cfg.BatchSize {
				if err := flush(); err != nil {
					return applied, err
				}
			}
		case <-flushTicker.C:
			// 时间到了,有就 flush(flush 内部 len(batch)==0 时 no-op)
			if err := flush(); err != nil {
				return applied, err
			}
		}
	}
}

// changeToMutation 把 ChangeEvent 翻译成 Sink.Mutation。
//
//	insert / update → Cols = After
//	delete         → Cols = Before（保留删除前快照供 audit / 反查）
//	Version        = EventTs（binlog 事件时间戳，乐观锁防"老 binlog 覆盖新值"）
func changeToMutation(c source.ChangeEvent) sink.Mutation {
	cols := c.After
	if c.Op == sink.OpDelete {
		cols = c.Before
	}
	return sink.Mutation{
		Op: c.Op, Table: c.Table, PK: c.PK,
		Cols: cols, Version: c.EventTs,
	}
}

// loadAllPartitionCheckpoints 按 (task_id, phase=incr, tableIdx) 拿全部 partition checkpoint。
// shard_no 编码 (tableIdx * ShardStride + partitionNo)；只返回属于当前 tableIdx 的 partitions。
func (e *InternalIncrEngine) loadAllPartitionCheckpoints(
	ctx context.Context, taskId int64, tableIdx, n int,
) ([]domain.Checkpoint, error) {
	list, err := e.ckptRepo.ListByTaskPhase(ctx, taskId, consts.PhaseIncr)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Checkpoint, n)
	for i := 0; i < n; i++ {
		result[i] = domain.Checkpoint{TaskId: taskId, Phase: consts.PhaseIncr, ShardNo: domain.EncodeShardNo(tableIdx, i)}
	}
	for _, c := range list {
		ti, pNo := domain.DecodeShardNo(c.ShardNo)
		if ti != tableIdx {
			continue
		}
		if pNo < 0 || pNo >= n {
			// partition 缩容场景：忽略超出 n 的 ckpt
			continue
		}
		result[pNo] = c
	}
	return result, nil
}

// minPartitionCkpt 选 CursorValue 最小（最旧）的那个 ckpt 作 IncrSubscribe 起点。
// 空 CursorValue（即首次启动该 partition）视为「比任何已写位点都早」，min 必须选它（否则会跳过该 partition 历史事件）。
func minPartitionCkpt(ckpts []domain.Checkpoint) domain.Checkpoint {
	if len(ckpts) == 0 {
		return domain.Checkpoint{}
	}
	result := ckpts[0]
	for _, c := range ckpts[1:] {
		if c.CursorValue == "" {
			// 空位点 = 该 partition 从未 flush；订阅必须从最早起点开始才能不丢这个 partition 的事件
			return c
		}
		if result.CursorValue == "" {
			continue
		}
		if compareBinlogPos(c.CursorValue, result.CursorValue) < 0 {
			result = c
		}
	}
	return result
}

// compareBinlogPos 比较两个 "file/pos" 字符串。
//
//	a > b  → +1
//	a == b →  0
//	a < b  → -1
//
// 比较规则：先比 file（字典序，假设文件名形如 mysql-bin.000001 按数字递增的 zero-padded 格式），再比 pos（数字）。
// 不合法格式回退到字符串比较（极端兼容性）。
func compareBinlogPos(a, b string) int {
	if a == b {
		return 0
	}
	fa, pa, oka := source.ParseBinlogPos(a)
	fb, pb, okb := source.ParseBinlogPos(b)
	if !oka || !okb {
		if a < b {
			return -1
		}
		return 1
	}
	if fa != fb {
		if fa < fb {
			return -1
		}
		return 1
	}
	if pa < pb {
		return -1
	}
	if pa > pb {
		return 1
	}
	return 0
}

func (e *InternalIncrEngine) updateCheckpointForPartition(
	ctx context.Context, taskId int64, tableIdx, partition int, binlogPos string,
) error {
	return e.ckptRepo.Save(ctx, domain.Checkpoint{
		TaskId:      taskId,
		Phase:       consts.PhaseIncr,
		ShardNo:     domain.EncodeShardNo(tableIdx, partition),
		CursorKind:  consts.CursorKindBinlog,
		CursorValue: binlogPos,
	})
}

// RunningTasks 枚举运行中任务（paused map 存的是运行中任务的 cancelFunc）。
func (e *InternalIncrEngine) RunningTasks() []int64 {
	var ids []int64
	e.paused.Range(func(k, _ any) bool {
		if id, ok := k.(int64); ok {
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

func (e *InternalIncrEngine) Pause(taskId int64) error {
	v, ok := e.paused.Load(taskId)
	if !ok {
		return fmt.Errorf("task %d not running", taskId)
	}
	// 占位窗口（LoadOrStore 占位 → context.WithCancel 替换为真 cancel）内可能拿到 nil。
	cancel, _ := v.(context.CancelFunc)
	if cancel == nil {
		return fmt.Errorf("task %d not running", taskId)
	}
	cancel()
	return nil
}

func (e *InternalIncrEngine) IsRunning(taskId int64) bool {
	_, ok := e.paused.Load(taskId)
	return ok
}

// updateDstLastEventTs 多 partition 并发 CAS：只保留 max(prev, candidate)。
// 实现细节：sync.Map.CompareAndSwap 比较的是 interface{} 值，candidate 必须是 int64 类型；
// 同 partition 串行调，跨 partition 用 CAS loop 兜底。
func (e *InternalIncrEngine) updateDstLastEventTs(taskId, candidate int64) {
	for {
		raw, loaded := e.dstLastEventTs.LoadOrStore(taskId, candidate)
		if !loaded {
			return // 首次写入成功
		}
		prev, ok := raw.(int64)
		if !ok {
			// 类型异常（不该发生）→ 直接覆盖
			e.dstLastEventTs.Store(taskId, candidate)
			return
		}
		if candidate <= prev {
			return // 已有更大值，无需更新
		}
		if e.dstLastEventTs.CompareAndSwap(taskId, raw, candidate) {
			return
		}
		// CAS 失败 → 其他 partition 更新过，重试
	}
}

// LagDst 实现 IncrEngine.LagDst。
func (e *InternalIncrEngine) LagDst(taskId int64) (int64, error) {
	if _, running := e.paused.Load(taskId); !running {
		return 0, fmt.Errorf("task %d not running", taskId)
	}
	raw, ok := e.dstLastEventTs.Load(taskId)
	if !ok {
		return -1, nil // 任务在跑但尚无 Apply 完成
	}
	ts, ok := raw.(int64)
	if !ok || ts <= 0 {
		return -1, nil
	}
	return time.Now().UnixMilli() - ts, nil
}

// Lag 通过 type assertion 拿 source.LagReporter（CanalSource 实现）。
// 多表 task 取所有表 Lag 的 max（最旧的延迟）。
func (e *InternalIncrEngine) Lag(taskId int64) (int64, error) {
	var maxLag int64 = -1
	found := false
	e.runningSources.Range(func(key, value any) bool {
		k, ok := key.(int64)
		if !ok {
			return true
		}
		// taskId 占高位（>> 16）
		if k>>16 != taskId {
			return true
		}
		r, ok := value.(source.LagReporter)
		if !ok {
			return true
		}
		found = true
		lag := r.Lag(taskId)
		if lag > maxLag {
			maxLag = lag
		}
		return true
	})
	if !found {
		// 检查是否在跑（即使没有 LagReporter 实现）
		_, taskRunning := e.paused.Load(taskId)
		if !taskRunning {
			return 0, fmt.Errorf("task %d not running", taskId)
		}
		return 0, fmt.Errorf("source does not implement LagReporter")
	}
	return maxLag, nil
}

// partitionOf 按 PK 算 partition 索引（同一 PK 恒落同一 partition，保证单行变更顺序）。
// 用 FNV-1a hash 而非裸取模有两个原因：
//   - PK 是任意 string（数值串 / Mongo ObjectID hex），裸取模需先数值化（ObjectID 无法），FNV 直接散列任意 string；
//   - 裸取模对低位与 n 相关的 PK 分布不均（如 PK 全是 n 的倍数 → 全落一桶），FNV 打散低位后均匀。
func partitionOf(pk string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(pk))
	return int(h.Sum32() % uint32(n))
}
