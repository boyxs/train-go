// Package verify 是对账引擎。
//
// 设计（architecture.md §8.3）：
//
//	Sample(rate)  按采样率扫 src + dst → 同 PK hash 采样池 → 比对 → 差异落 validate_log；
//	Full          = Sample(1.0)；
//	Repair        按 strategy 修复指定 mismatch IDs；当前实现仅 RepairMarkOnly（标记不动数据）。
//
// 边界：
//   - src/dst overwrite 策略需要"读 + 写"组合接口，Sink 当前只写；
//   - 同构 MySQL → MySQL；异构对账支持 mysql / es / mongo（BuildDst 按 SinkType 分发 +
//     normalizeRows 比对前对两侧过表的 transform 归一 + 剥 sink 注入元数据 _id/version，详见 sinkInjectedFields）。
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"

	"github.com/webook/migrator/consts"
	"github.com/webook/migrator/domain"
	migratorerrs "github.com/webook/migrator/errs"
	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/pipeline/transform"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/service"
	"github.com/webook/pkg/errs"
	"github.com/webook/pkg/logger"
)

// RepairStrategy mismatch 修复策略。
type RepairStrategy string

const (
	// RepairSrcOverwriteDst 用源端数据覆盖目标端（src → dst 单向修复，最常用）。
	RepairSrcOverwriteDst RepairStrategy = "src_overwrite_dst"
	// RepairDstOverwriteSrc 用目标端数据覆盖源端（dst → src 反向修复，少见；cutover 后期 NEW 成新真值时手动用）。
	RepairDstOverwriteSrc RepairStrategy = "dst_overwrite_src"
	// RepairMarkOnly 不动数据，仅在 validate_log 标记 repaired=1（处理"差异业务可接受"的人工判定结果）。
	RepairMarkOnly RepairStrategy = "mark_only"
)

// ErrInvalidValidateLog mismatch 记录的 diff_detail 字段缺失或损坏。
var ErrInvalidValidateLog = errs.New(500, "validate_log diff_detail 字段缺失或损坏").WithReason("MIGRATOR_VALIDATE_LOG_INVALID")

// 采样率非法复用 migrator/errs.ErrInvalidSampleRate（同一错误，去重）。

// VerifyEngine 对账引擎接口。
type VerifyEngine interface {
	// Sample 按采样率扫描 src/dst 同 PK 采样池比对；返回差异 count（已落 validate_log）。
	Sample(ctx context.Context, taskId int64, rate float64) (int64, error)
	// Full = Sample(1.0)。
	Full(ctx context.Context, taskId int64) (int64, error)
	// Repair 按 strategy 修复指定 mismatch IDs；返回成功修复数。
	Repair(ctx context.Context, taskId int64, strategy RepairStrategy, ids []int64) (int64, error)
	// ListMismatch 分页列未修复的对账差异（created_at ASC，最老优先）。
	ListMismatch(ctx context.Context, taskId int64, offset, limit int) ([]domain.ValidateLog, int64, error)
}

// Config VerifyEngine 行为参数。
type Config struct {
	BatchSize  int // FullScan 单批行数 + validate_log BatchInsert 大小
	ChannelBuf int
}

type InternalVerifyEngine struct {
	taskSvc      service.TaskService
	validateRepo repository.ValidateLogRepository
	srcFactory   source.SourceFactory
	sinkFactory  sink.SinkFactory
	transformReg *transform.Registry
	l            logger.LoggerX
	cfg          Config
}

func NewVerifyEngine(
	taskSvc service.TaskService,
	validateRepo repository.ValidateLogRepository,
	srcFactory source.SourceFactory,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
	transformReg *transform.Registry,
	cfg Config,
) VerifyEngine {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1000
	}
	if cfg.ChannelBuf <= 0 {
		cfg.ChannelBuf = 4096
	}
	return &InternalVerifyEngine{
		taskSvc: taskSvc, validateRepo: validateRepo,
		srcFactory: srcFactory, sinkFactory: sinkFactory,
		transformReg: transformReg,
		l:            l, cfg: cfg,
	}
}

// buildPipes 按 task + tableIdx 构造本次跑的 4 个 Source/Sink + 业务表名。
// Sample / Full / Repair 共用。
func (e *InternalVerifyEngine) buildPipes(ctx context.Context, task domain.Task, tableIdx int) (
	srcSrc, dstSrc source.FullSource, srcSnk, dstSnk sink.Sink, tableName string, err error,
) {
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("pick table %d: %w", tableIdx, err)
	}
	srcSrc, err = e.srcFactory.BuildFullSrc(ctx, task, tableIdx)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("build src source: %w", err)
	}
	dstSrc, err = e.srcFactory.BuildDst(ctx, task, tableIdx)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("build dst source: %w", err)
	}
	srcSnk, err = e.sinkFactory.BuildSrc(ctx, task, tableIdx)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("build src sink: %w", err)
	}
	dstSnk, err = e.sinkFactory.BuildDst(ctx, task, tableIdx)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("build dst sink: %w", err)
	}
	return srcSrc, dstSrc, srcSnk, dstSnk, tm.Src, nil
}

func (e *InternalVerifyEngine) Sample(ctx context.Context, taskId int64, rate float64) (int64, error) {
	if rate <= 0 || rate > 1 {
		return 0, migratorerrs.ErrInvalidSampleRate
	}
	task, err := e.taskSvc.Get(ctx, taskId)
	if err != nil {
		return 0, fmt.Errorf("find task: %w", err)
	}
	tables, err := task.Tables()
	if err != nil {
		return 0, err
	}
	// 多表对账：每张表独立 Sample，累加 mismatch
	var totalMismatch int64
	for tableIdx := range tables {
		count, err := e.sampleTable(ctx, task, tableIdx, rate)
		if err != nil {
			return totalMismatch, fmt.Errorf("table %d: %w", tableIdx, err)
		}
		totalMismatch += count
	}
	return totalMismatch, nil
}

// sampleTable 单张表的采样对账。
func (e *InternalVerifyEngine) sampleTable(ctx context.Context, task domain.Task, tableIdx int, rate float64) (int64, error) {
	taskId := task.Id
	srcSrc, dstSrc, _, _, tableName, err := e.buildPipes(ctx, task, tableIdx)
	if err != nil {
		return 0, err
	}
	tm, err := task.PickTable(tableIdx)
	if err != nil {
		return 0, err
	}
	// 异构对账：拿本表的 transform，比对前把两侧归一到同形态（见 diffAndLog）
	tf, err := e.transformReg.Get(tm.Transform)
	if err != nil {
		return 0, fmt.Errorf("resolve transform: %w", err)
	}

	// 用 PK 的 FNV hash 决定采样池：src + dst 同一规则 → 采样集合一致；string PK 直接散列（数值串 / ObjectID 通吃）
	sampleScale := int64(rate * 1e6)
	inSample := func(pk string) bool {
		h := fnv.New64a()
		_, _ = h.Write([]byte(pk))
		return h.Sum64()%1_000_000 < uint64(sampleScale)
	}

	shard := source.ShardSpec{No: 0, PKMin: 1, PKMax: 1 << 62, BatchSz: e.cfg.BatchSize}

	srcRows := map[string]map[string]any{}
	dstRows := map[string]map[string]any{}

	srcCh := make(chan source.Row, e.cfg.ChannelBuf)
	dstCh := make(chan source.Row, e.cfg.ChannelBuf)
	srcErr := make(chan error, 1)
	dstErr := make(chan error, 1)
	go func() {
		defer close(srcCh)
		srcErr <- srcSrc.FullScan(ctx, shard, srcCh)
	}()
	go func() {
		defer close(dstCh)
		dstErr <- dstSrc.FullScan(ctx, shard, dstCh)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	var mu sync.Mutex
	go func() {
		defer wg.Done()
		for r := range srcCh {
			if inSample(r.PK) {
				mu.Lock()
				srcRows[r.PK] = r.Cols
				mu.Unlock()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for r := range dstCh {
			if inSample(r.PK) {
				mu.Lock()
				dstRows[r.PK] = r.Cols
				mu.Unlock()
			}
		}
	}()
	wg.Wait()
	if err := <-srcErr; err != nil {
		return 0, fmt.Errorf("src scan: %w", err)
	}
	if err := <-dstErr; err != nil {
		return 0, fmt.Errorf("dst scan: %w", err)
	}

	return e.diffAndLog(ctx, taskId, tableName, srcRows, dstRows, tf)
}

func (e *InternalVerifyEngine) Full(ctx context.Context, taskId int64) (int64, error) {
	return e.Sample(ctx, taskId, 1.0)
}

// diffAndLog 对比 src/dst 行表 → 落 validate_log，返回 mismatch 数。
//
// 三类差异：
//
//	missing  src 有，dst 无
//	extra    dst 有，src 无
//	diff     都有但字段不一致（按 JSON marshal 比较，保留 diff_fields 列表）
func (e *InternalVerifyEngine) diffAndLog(ctx context.Context, taskId int64, tableName string, srcRows, dstRows map[string]map[string]any, tf transform.Transformer) (int64, error) {
	// 异构对账：比对前对两侧应用 transform 归一到同形态（对已正确迁移的 dst 是幂等的）+ 去 _id
	// （_id 是 PK 回显——Mongo 文档主键 / MongoSink 注入；行已按 PK 匹配，不参与字段比对）。
	nsrc, err := normalizeRows(tf, tableName, srcRows)
	if err != nil {
		return 0, fmt.Errorf("normalize src rows: %w", err)
	}
	ndst, err := normalizeRows(tf, tableName, dstRows)
	if err != nil {
		return 0, fmt.Errorf("normalize dst rows: %w", err)
	}

	logs := make([]domain.ValidateLog, 0)

	for pk, srcRow := range nsrc {
		dstRow, ok := ndst[pk]
		if !ok {
			logs = append(logs, e.makeLog(taskId, tableName, pk, consts.MismatchKindMissing, srcRow, nil, nil))
			continue
		}
		if diff := diffFields(srcRow, dstRow); len(diff) > 0 {
			logs = append(logs, e.makeLog(taskId, tableName, pk, consts.MismatchKindDiff, srcRow, dstRow, diff))
		}
	}
	for pk, dstRow := range ndst {
		if _, ok := nsrc[pk]; !ok {
			logs = append(logs, e.makeLog(taskId, tableName, pk, consts.MismatchKindExtra, nil, dstRow, nil))
		}
	}

	if err := e.validateRepo.BatchInsert(ctx, logs); err != nil {
		return 0, fmt.Errorf("validate_log batch insert: %w", err)
	}
	e.l.Info("verify done",
		logger.Int64("task_id", taskId),
		logger.Int("mismatch_count", len(logs)))
	return int64(len(logs)), nil
}

func (e *InternalVerifyEngine) makeLog(taskId int64, tableName string, pk string, kind string, src, dst map[string]any, diffFieldList []string) domain.ValidateLog {
	detail := map[string]any{}
	if src != nil {
		detail["src"] = src
	}
	if dst != nil {
		detail["dst"] = dst
	}
	if len(diffFieldList) > 0 {
		detail["diff_fields"] = diffFieldList
	}
	// detail 是 map[string]any with primitive values，理论 marshal 不可能失败；兜底走 Sprintf 保审计有迹可循
	raw, err := json.Marshal(detail)
	if err != nil {
		raw = []byte(fmt.Sprintf("marshal failed: %v, detail=%+v", err, detail))
	}
	return domain.ValidateLog{
		TaskId:       taskId,
		Direction:    consts.DirectionSrcToDst,
		BizTable:     tableName,
		BizId:        pk,
		MismatchKind: kind,
		DiffDetail:   string(raw),
	}
}

// diffFields 返回 src/dst 不一致的字段名列表（按字段名字典序）。两行字段集差 + 值差都算 diff。
func diffFields(src, dst map[string]any) []string {
	keys := map[string]struct{}{}
	for k := range src {
		keys[k] = struct{}{}
	}
	for k := range dst {
		keys[k] = struct{}{}
	}
	var diff []string
	for k := range keys {
		// 用 JSON marshal 比较防止 int vs int64 / []byte vs string 等驱动层类型差异误判
		sv, sverr := json.Marshal(src[k])
		dv, dverr := json.Marshal(dst[k])
		// 任一 marshal 失败视为有差异（marshal 不上的字段一律标 diff 让人工排查）
		if sverr != nil || dverr != nil || string(sv) != string(dv) {
			diff = append(diff, k)
		}
	}
	sort.Strings(diff)
	return diff
}

// sinkInjectedFields 是 Sink 写 dst 时注入的元数据键（MongoSink 的 _id PK 回显 / version 乐观锁），
// verify 比对时两侧剥掉避免假阳性。业务表用同名列需避开。
var sinkInjectedFields = map[string]struct{}{
	"_id":     {},
	"version": {},
}

// normalizeRows 对每行应用 transform 归一 + 剥 sinkInjectedFields；同构（Identity + 无注入键）等价原样。
func normalizeRows(tf transform.Transformer, tableName string, rows map[string]map[string]any) (map[string]map[string]any, error) {
	out := make(map[string]map[string]any, len(rows))
	for pk, cols := range rows {
		m, terr := tf.Transform(sink.Mutation{Op: sink.OpInsert, Table: tableName, PK: pk, Cols: cols})
		if terr != nil {
			return nil, fmt.Errorf("transform pk %q: %w", pk, terr)
		}
		nc := make(map[string]any, len(m.Cols))
		for k, v := range m.Cols {
			if _, skip := sinkInjectedFields[k]; skip {
				continue
			}
			nc[k] = v
		}
		out[pk] = nc
	}
	return out, nil
}

func (e *InternalVerifyEngine) ListMismatch(ctx context.Context, taskId int64, offset, limit int) ([]domain.ValidateLog, int64, error) {
	return e.validateRepo.ListUnrepaired(ctx, taskId, offset, limit)
}

func (e *InternalVerifyEngine) Repair(ctx context.Context, taskId int64, strategy RepairStrategy, ids []int64) (int64, error) {
	task, err := e.taskSvc.Get(ctx, taskId)
	if err != nil {
		return 0, fmt.Errorf("find task: %w", err)
	}
	switch strategy {
	case RepairMarkOnly:
		if err := e.validateRepo.MarkRepaired(ctx, ids); err != nil {
			return 0, fmt.Errorf("mark repaired: %w", err)
		}
		return int64(len(ids)), nil
	case RepairSrcOverwriteDst, RepairDstOverwriteSrc:
		// Repair 跨表场景：用 validate_log.biz_table 反查 task tables 索引，按表分组后调对应 sink
		return e.repairAcrossTables(ctx, task, strategy, ids)
	default:
		return 0, fmt.Errorf("unknown repair strategy %q", strategy)
	}
}

// repairAcrossTables 按 validate_log.biz_table 把 ids 分组到对应 tableIdx，每组用各自的 Sink 修复。
// strategy = src_overwrite_dst → 用 dstSink；dst_overwrite_src → 用 srcSink。
func (e *InternalVerifyEngine) repairAcrossTables(
	ctx context.Context, task domain.Task, strategy RepairStrategy, ids []int64,
) (int64, error) {
	logs, err := e.validateRepo.FindByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("find validate_log: %w", err)
	}
	tables, err := task.Tables()
	if err != nil {
		return 0, err
	}
	// table name → tableIdx 反查
	tableNameToIdx := make(map[string]int, len(tables))
	for i, tm := range tables {
		tableNameToIdx[tm.Src] = i
	}
	// 按 tableIdx 分组 logs
	groups := map[int][]domain.ValidateLog{}
	for _, lg := range logs {
		ti, ok := tableNameToIdx[lg.BizTable]
		if !ok {
			e.l.Warn("repair: validate_log biz_table not in task tables",
				logger.Int64("validate_log_id", lg.Id),
				logger.String("biz_table", lg.BizTable))
			continue
		}
		groups[ti] = append(groups[ti], lg)
	}
	var totalRepaired int64
	for tableIdx, glogs := range groups {
		_, _, srcSnk, dstSnk, _, err := e.buildPipes(ctx, task, tableIdx)
		if err != nil {
			return totalRepaired, err
		}
		var snk sink.Sink
		var snapKey string
		if strategy == RepairSrcOverwriteDst {
			snk = dstSnk
			snapKey = "src"
		} else {
			snk = srcSnk
			snapKey = "dst"
		}
		n, err := e.applyRepairBatch(ctx, glogs, snapKey, snk)
		totalRepaired += n
		if err != nil {
			return totalRepaired, err
		}
	}
	return totalRepaired, nil
}

// applyRepairBatch 把一组 validate_log 转 Mutation → Sink.Apply → MarkRepaired。
// missing/extra/diff 处理详见 buildRepairMutation 注释。
func (e *InternalVerifyEngine) applyRepairBatch(
	ctx context.Context, logs []domain.ValidateLog, snapshotKey string, snk sink.Sink,
) (int64, error) {
	var mutations []sink.Mutation
	var successIDs []int64
	for _, lg := range logs {
		mut, mutErr := buildRepairMutation(lg, snapshotKey)
		if mutErr != nil {
			e.l.Warn("repair: build mutation failed",
				logger.Int64("validate_log_id", lg.Id),
				logger.String("kind", lg.MismatchKind),
				logger.Error(mutErr))
			continue
		}
		mutations = append(mutations, mut)
		successIDs = append(successIDs, lg.Id)
	}
	if len(mutations) == 0 {
		return 0, nil
	}
	if err := snk.Apply(ctx, mutations); err != nil {
		return 0, fmt.Errorf("repair sink apply: %w", err)
	}
	if err := e.validateRepo.MarkRepaired(ctx, successIDs); err != nil {
		e.l.Warn("repair: mark_repaired failed (sink apply already succeeded)",
			logger.Int("ids_count", len(successIDs)), logger.Error(err))
	}
	e.l.Info("repair done",
		logger.String("snapshot_key", snapshotKey),
		logger.Int64("repaired", int64(len(successIDs))))
	return int64(len(successIDs)), nil
}

// buildRepairMutation 把 validate_log 行翻译成 Sink Mutation。
//
// snapshotKey 决定从 diff_detail 中拿哪一侧（"src" 用于 src_overwrite_dst，"dst" 用于反向）。
//
// diff_detail 结构（diffAndLog 写入）：
//
//	{ "src": {col1: val1, ...}, "dst": {col1: val1, ...}, "diff_fields": [...] }
func buildRepairMutation(lg domain.ValidateLog, snapshotKey string) (sink.Mutation, error) {
	var detail map[string]any
	if err := json.Unmarshal([]byte(lg.DiffDetail), &detail); err != nil {
		return sink.Mutation{}, fmt.Errorf("%w: unmarshal: %v", ErrInvalidValidateLog, err)
	}
	snap, ok := detail[snapshotKey].(map[string]any)
	// missing/extra 场景：snapshotKey 对应的一侧没数据
	if !ok {
		// 用 snapshotKey 反向作 op：
		//   snapshotKey=src 时 src 没数据 → 删 dst（extra 场景）
		//   snapshotKey=dst 时 dst 没数据 → 删 src（missing 场景）
		return sink.Mutation{
			Op:    sink.OpDelete,
			Table: lg.BizTable,
			PK:    lg.BizId,
		}, nil
	}
	// 两侧都有（diff）或 snapshotKey 这侧有（missing/extra 反向）→ upsert snapshot
	return sink.Mutation{
		Op:    sink.OpInsert,
		Table: lg.BizTable,
		PK:    lg.BizId,
		Cols:  snap,
	}, nil
}
