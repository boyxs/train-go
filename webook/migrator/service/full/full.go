// Package full 是全量同步引擎。
//
// 设计（architecture.md §8.3 + §14 任务 #3）：
//
//	Run     按 task.Tables() 迭代每张表 → PKRange 切片 → 并发跑 ShardSpec[] → 攒批写 Sink → checkpoint 续传；
//	Pause   取消正在跑的 task（含 fan-out 的所有表）；
//
// 多表设计：
//   - task 可承载 N 张表（task.TablesJSON 数组）
//   - 引擎内部按 tables[*] 迭代，每张表独立 build src/snk + 跑分片
//   - 每张表的 checkpoint shard_no 编码 (tableIdx * domain.ShardStride + shardNo)
//   - hintShards 非空时仅对单表 task 生效；多表 task 强制走 PKRanger 自动切片
//
// 工厂动态调度：构造时持 SourceFactory / SinkFactory；Run 入口按 task + tableIdx 构造 Source/Sink。
package full

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/webook/migrator/consts"
	"github.com/webook/migrator/domain"
	migratorerrs "github.com/webook/migrator/errs"
	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/pipeline/transform"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/service"
	"github.com/webook/pkg/logger"
)

const (
	DefaultBatchSize  = 1000
	DefaultChannelBuf = 4096
	DefaultShardCount = 16
)

// FullEngine 全量同步引擎接口。
type FullEngine interface {
	// Run 启动指定 task 的全量同步。
	// task 单表时使用 hintShards（如提供）；多表时忽略 hintShards 自动按每张表 PKRange 切片。
	// 所有表的所有分片完成后返回 nil；任一失败或 ctx cancel 提前返回。
	// 如果同 taskId 已在运行，直接返回 migratorerrs.ErrTaskAlreadyRunning（防并发双开）。
	Run(ctx context.Context, taskId int64, hintShards []source.ShardSpec) error
	// Pause 取消 Run 中的 task；非阻塞。
	Pause(taskId int64) error
	// IsRunning 判断 taskId 当前是否有 Run 正在执行。handler 层早期判 409 用。
	IsRunning(taskId int64) bool
}

// Config FullEngine 行为参数。
type Config struct {
	BatchSize  int // 每批攒到多少行触发 Sink.Apply
	ChannelBuf int // Source → consumer 的 Row chan 缓冲
	ShardCount int // 自动切片时每张表的目标 shard 数（默认 16）
}

type InternalFullEngine struct {
	taskSvc      service.TaskService
	ckptRepo     repository.CheckpointRepository
	srcFactory   source.SourceFactory
	sinkFactory  sink.SinkFactory
	transformReg *transform.Registry
	l            logger.LoggerX
	cfg          Config

	paused sync.Map // map[int64]context.CancelFunc — 注册 Run 中的 cancel
}

func NewFullEngine(
	taskSvc service.TaskService,
	ckptRepo repository.CheckpointRepository,
	srcFactory source.SourceFactory,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
	transformReg *transform.Registry,
	cfg Config,
) FullEngine {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.ChannelBuf <= 0 {
		cfg.ChannelBuf = DefaultChannelBuf
	}
	if cfg.ShardCount <= 0 {
		cfg.ShardCount = DefaultShardCount
	}
	return &InternalFullEngine{
		taskSvc: taskSvc, ckptRepo: ckptRepo,
		srcFactory: srcFactory, sinkFactory: sinkFactory,
		transformReg: transformReg,
		l:            l, cfg: cfg,
	}
}

func (e *InternalFullEngine) Run(ctx context.Context, taskId int64, hintShards []source.ShardSpec) error {
	// 占位防 race：LoadOrStore 原子操作，已在跑直接返 ErrTaskAlreadyRunning。
	// 占位值用 nil cancel；下面 context.WithCancel 后会被真 cancel 覆盖。
	if _, loaded := e.paused.LoadOrStore(taskId, context.CancelFunc(nil)); loaded {
		return migratorerrs.ErrTaskAlreadyRunning
	}
	// 占位 + 真 cancel 都靠这一个 defer 清掉。
	defer e.paused.Delete(taskId)

	// 标记 FullRunning；失败仅 Warn 不阻塞（status 是冗余可观测字段，写失败不影响业务）
	if err := e.taskSvc.UpdateStatus(ctx, taskId, domain.TaskStatusFullRunning); err != nil {
		e.l.Warn("update task status to full_running failed",
			logger.Int64("task_id", taskId), logger.Error(err))
	}
	// defer 统一收口：成功 → FullDone，失败 → Failed。
	// 用 context.Background()：避免 Run 因 Pause 被 cancel 时连状态更新都做不到。
	var runErr error
	defer func() {
		final := domain.TaskStatusFullDone
		if runErr != nil {
			final = domain.TaskStatusFailed
		}
		if err := e.taskSvc.UpdateStatus(context.Background(), taskId, final); err != nil {
			e.l.Warn("update task final status failed",
				logger.Int64("task_id", taskId),
				logger.Int("final_status", int(final)),
				logger.Error(err))
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
	for tableIdx := range tables {
		ti := tableIdx
		src, err := e.srcFactory.BuildFullSrc(runCtx, task, ti)
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

		// 多表 task 强制自动切片；单表 task 优先使用 hintShards
		shards := hintShards
		if len(tables) > 1 || len(shards) == 0 {
			shards, err = e.resolveShards(runCtx, src)
			if err != nil {
				runErr = fmt.Errorf("resolve shards for task %d table %d: %w", taskId, ti, err)
				return runErr
			}
		}

		for _, shard := range shards {
			sh := shard
			tiCaptured := ti
			srcCaptured := src
			snkCaptured := snk
			tfCaptured := tf
			g.Go(func() error {
				return e.runShard(gctx, taskId, tiCaptured, sh, srcCaptured, snkCaptured, tfCaptured)
			})
		}
	}
	if err := g.Wait(); err != nil {
		e.l.Warn("full engine run aborted",
			logger.Int64("task_id", taskId), logger.Error(err))
		runErr = err
		return runErr
	}
	e.l.Info("full engine run done",
		logger.Int64("task_id", taskId),
		logger.Int("tables", len(tables)))
	return nil
}

// resolveShards 按 Source 是否实现 PKRanger 决定切片：
// 实现 → PKRange + PlanShards；不实现 → 单 shard 兜底（[1, 1<<62]）。
func (e *InternalFullEngine) resolveShards(ctx context.Context, src source.FullSource) ([]source.ShardSpec, error) {
	ranger, ok := src.(source.PKRanger)
	if !ok {
		return []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 1 << 62}}, nil
	}
	minPK, maxPK, err := ranger.PKRange(ctx)
	if err != nil {
		return nil, err
	}
	if minPK == 0 && maxPK == 0 {
		return []source.ShardSpec{{No: 0, PKMin: 0, PKMax: 0}}, nil
	}
	return PlanShards(minPK, maxPK, e.cfg.ShardCount), nil
}

// runShard 单 (table, shard) 的流水线：Source.FullScan → Row chan → 攒批 → Sink.Apply → checkpoint 持久化。
func (e *InternalFullEngine) runShard(
	ctx context.Context, taskId int64, tableIdx int, shard source.ShardSpec,
	src source.FullSource, snk sink.Sink, tf transform.Transformer,
) error {
	out := make(chan source.Row, e.cfg.ChannelBuf)
	scanErrCh := make(chan error, 1)
	go func() {
		defer close(out)
		scanErrCh <- src.FullScan(ctx, shard, out)
	}()

	batch := make([]sink.Mutation, 0, e.cfg.BatchSize)
	var lastPK string // 最后发出的 PK：源按 PK 升序流式扫描，last 即游标位
	var totalApplied int64
	encodedShardNo := domain.EncodeShardNo(tableIdx, shard.No)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := snk.Apply(ctx, batch); err != nil {
			return fmt.Errorf("sink apply task %d table %d shard %d: %w", taskId, tableIdx, shard.No, err)
		}
		totalApplied += int64(len(batch))
		if err := e.updateCheckpoint(ctx, taskId, encodedShardNo, lastPK); err != nil {
			return fmt.Errorf("update checkpoint task %d table %d shard %d: %w", taskId, tableIdx, shard.No, err)
		}
		batch = batch[:0]
		return nil
	}

	for row := range out {
		mut, terr := tf.Transform(sink.Mutation{
			Op:    sink.OpInsert,
			Table: row.Table,
			PK:    row.PK,
			Cols:  row.Cols,
		})
		if terr != nil {
			return fmt.Errorf("transform task %d table %d shard %d pk %q: %w", taskId, tableIdx, shard.No, row.PK, terr)
		}
		batch = append(batch, mut)
		// 游标 = 最后发出的 PK：源按 PK 升序流式扫描，故最后一行即本批游标位（数值串 / Mongo ObjectID 通吃）。
		lastPK = row.PK
		if len(batch) >= e.cfg.BatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if err := <-scanErrCh; err != nil {
		return fmt.Errorf("source full scan task %d table %d shard %d: %w", taskId, tableIdx, shard.No, err)
	}
	e.l.Info("full shard done",
		logger.Int64("task_id", taskId),
		logger.Int("table_idx", tableIdx),
		logger.Int("shard", shard.No),
		logger.Int64("applied", totalApplied))
	return nil
}

func (e *InternalFullEngine) updateCheckpoint(ctx context.Context, taskId int64, encodedShardNo int32, lastPK string) error {
	return e.ckptRepo.Save(ctx, domain.Checkpoint{
		TaskId:      taskId,
		Phase:       consts.PhaseFull,
		ShardNo:     encodedShardNo,
		CursorKind:  consts.CursorKindIDRange,
		CursorValue: lastPK,
	})
}

func (e *InternalFullEngine) Pause(taskId int64) error {
	v, ok := e.paused.Load(taskId)
	if !ok {
		return fmt.Errorf("task %d not running", taskId)
	}
	// 占位窗口（LoadOrStore 占位 → context.WithCancel 替换为真 cancel）内可能拿到 nil；
	// 视为"任务刚起步还没建 ctx"，等下次 Pause 即可。
	cancel, _ := v.(context.CancelFunc)
	if cancel == nil {
		return fmt.Errorf("task %d not running", taskId)
	}
	cancel()
	return nil
}

func (e *InternalFullEngine) IsRunning(taskId int64) bool {
	_, ok := e.paused.Load(taskId)
	return ok
}

// PlanShards 把 [min, max] 等分成 count 片（含端点）。
// 用于任务启动时根据 Source.PKRange 查到的范围切片。
func PlanShards(min, max int64, count int) []source.ShardSpec {
	if count <= 0 {
		count = 1
	}
	if max < min {
		return nil
	}
	total := max - min + 1
	if int64(count) > total {
		count = int(total)
	}
	step := total / int64(count)
	shards := make([]source.ShardSpec, 0, count)
	for i := 0; i < count; i++ {
		pkMin := min + int64(i)*step
		pkMax := pkMin + step - 1
		if i == count-1 {
			pkMax = max
		}
		shards = append(shards, source.ShardSpec{No: i, PKMin: pkMin, PKMax: pkMax})
	}
	return shards
}
