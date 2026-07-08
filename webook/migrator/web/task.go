package web

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/migrator/domain"
	migratorerrs "github.com/boyxs/train-go/webook/migrator/errs"
	"github.com/boyxs/train-go/webook/migrator/pipeline/dsn"
	"github.com/boyxs/train-go/webook/migrator/pipeline/source"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/service"
	"github.com/boyxs/train-go/webook/migrator/service/full"
	"github.com/boyxs/train-go/webook/migrator/service/incr"
	"github.com/boyxs/train-go/webook/migrator/service/replay"
	"github.com/boyxs/train-go/webook/migrator/service/switching"
	"github.com/boyxs/train-go/webook/migrator/service/verify"
	"github.com/boyxs/train-go/webook/pkg/errs"
	"github.com/boyxs/train-go/webook/pkg/ginx"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// TaskHandler 是 migrator 控制台所有端点的入口（CRUD + 运行时控制）。
//
// 端点矩阵（14 个，对齐 architecture.md §6）：
//
//	CRUD       POST /tasks · GET /tasks · GET /tasks/:id
//	Lifecycle  POST /preflight · POST /tasks/:id/{start,pause,throttle,gray,switch,verify,repair,replay-dl}
//	Query      GET /tasks/:id/{lag,mismatch}
//
// 引擎类字段（fullEng / incrEng / verEng 等）支持 nil — 调用前判 nil 返 501，
// 允许 wire 按需注入（未注入的引擎对应 endpoint 返 501 而非 panic）。
type TaskHandler interface {
	RegisterRoutes(r *gin.Engine)
}

type InternalTaskHandler struct {
	svc        service.TaskService
	fullEng    full.FullEngine
	incrEng    incr.IncrEngine
	verEng     verify.VerifyEngine
	swSvc      switching.SwitchService
	replaySvc  replay.ReplayService // 死信重放；可空 → endpoint 返 501
	srcFactory source.SourceFactory // Start 时 PKRange 自动切片按 task 动态 build
	resolver   dsn.Resolver         // Preflight 按 sourceDsnRef 拿源库 *gorm.DB 真查 binlog_format / 表 PK
	l          logger.LoggerX
}

func NewTaskHandler(
	s service.TaskService,
	fe full.FullEngine,
	ie incr.IncrEngine,
	ve verify.VerifyEngine,
	ss switching.SwitchService,
	replaySvc replay.ReplayService,
	srcFactory source.SourceFactory,
	resolver dsn.Resolver,
	l logger.LoggerX,
) TaskHandler {
	if l == nil {
		l = logger.NewNopLogger()
	}
	return &InternalTaskHandler{
		svc: s, fullEng: fe, incrEng: ie, verEng: ve,
		swSvc:      ss,
		replaySvc:  replaySvc,
		srcFactory: srcFactory,
		resolver:   resolver,
		l:          l,
	}
}

func (h *InternalTaskHandler) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/migrator")
	// ── CRUD ─────────────────────────
	g.POST("/tasks", ginx.WrapReq[createReq](h.Create))
	g.GET("/tasks", ginx.Wrap(h.List))
	g.GET("/tasks/:id", ginx.Wrap(h.Get))
	// ── Lifecycle 运行时控制 ──────────
	g.POST("/preflight", ginx.WrapReq[preflightReq](h.Preflight))
	g.POST("/tasks/:id/start", ginx.WrapReq[startReq](h.Start))
	g.POST("/tasks/:id/pause", ginx.Wrap(h.Pause))
	g.POST("/tasks/:id/throttle", ginx.WrapReq[throttleReq](h.Throttle))
	g.POST("/tasks/:id/gray", ginx.WrapReq[grayReq](h.SetGray))
	g.POST("/tasks/:id/switch", ginx.WrapReq[switchReq](h.SetSwitch))
	g.GET("/tasks/:id/lag", ginx.Wrap(h.Lag))
	g.POST("/tasks/:id/verify", ginx.WrapReq[verifyReq](h.Verify))
	g.GET("/tasks/:id/mismatch", ginx.Wrap(h.Mismatch))
	g.POST("/tasks/:id/repair", ginx.WrapReq[repairReq](h.Repair))
	g.POST("/tasks/:id/replay-dl", ginx.WrapReq[replayReq](h.ReplayDL))
}

// errEngineNotConfigured 引擎未通过 wire 装配。
var errEngineNotConfigured = errs.New(501, "engine not configured (wire 未注入引擎实现)").WithReason("MIGRATOR_ENGINE_NOT_CONFIGURED")

// taskIdFromPath 从 path :id 提取 taskId。
func taskIdFromPath(ctx *gin.Context) (int64, error) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, migratorerrs.ErrInvalidArgument.WithCause(err)
	}
	return id, nil
}

// ═════════════════════════════════════════════════════════════
// CRUD（Create / Get / List）
// ═════════════════════════════════════════════════════════════

type createReq struct {
	Name         string                `json:"name" binding:"required,min=1,max=128"`
	Mode         string                `json:"mode" binding:"required,oneof=dual_write cdc"`
	Kind         string                `json:"kind" binding:"required,oneof=cross_dc sharding schema heterogeneous"`
	SourceType   string                `json:"sourceType" binding:"omitempty,oneof=mysql mongo"`
	SourceDsnRef string                `json:"sourceDsnRef" binding:"required"`
	SinkType     string                `json:"sinkType" binding:"required,oneof=mysql es clickhouse mongo tidb kafka"`
	SinkDsnRef   string                `json:"sinkDsnRef" binding:"required"`
	Tables       []domain.TableMapping `json:"tables" binding:"required,min=1"`
}

func (h *InternalTaskHandler) Create(ctx *gin.Context, req createReq) (Result, error) {
	id, err := h.svc.Create(ctx.Request.Context(), service.CreateReq{
		Name: req.Name, Mode: domain.Mode(req.Mode), Kind: domain.Kind(req.Kind),
		SourceType: domain.SourceType(req.SourceType), SourceDsnRef: req.SourceDsnRef, SinkType: req.SinkType, SinkDsnRef: req.SinkDsnRef,
		Tables: req.Tables,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Data: gin.H{"taskId": id}}, nil
}

func (h *InternalTaskHandler) Get(ctx *gin.Context) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	t, err := h.svc.Get(ctx.Request.Context(), id)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: toTaskVO(t)}, nil
}

func (h *InternalTaskHandler) List(ctx *gin.Context) (Result, error) {
	offset, err := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		return Result{}, errs.New(400, "offset 必须是非负整数")
	}
	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	if err != nil {
		return Result{}, errs.New(400, "limit 必须是整数")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	opts := repository.ListOpts{Offset: offset, Limit: limit}
	if s := ctx.Query("status"); s != "" {
		v, err := strconv.ParseInt(s, 10, 8)
		if err != nil {
			return Result{}, errs.New(400, "status 必须是整数")
		}
		st := domain.TaskStatus(v)
		opts.Status = &st
	}
	list, total, err := h.svc.List(ctx.Request.Context(), opts)
	if err != nil {
		return Result{}, err
	}
	vos := make([]TaskVO, len(list))
	for i, t := range list {
		vos[i] = toTaskVO(t)
	}
	return Result{Data: ginx.PageResult{List: vos, Total: total}}, nil
}

// ═════════════════════════════════════════════════════════════
// Lifecycle 控制（preflight / start / pause / throttle / gray / switch / lag / verify / mismatch / repair / replay-dl）
// ═════════════════════════════════════════════════════════════

// ── /preflight ──────────────────────────────────────────────
type preflightReq struct {
	SourceDsnRef string   `json:"sourceDsnRef" binding:"required"`
	Tables       []string `json:"tables" binding:"required,min=1"`
}

// Preflight 飞行前检查：源库 binlog_format=ROW / gtid_mode=ON / 每张表有 PK。
//
// 实现细节：
//   - 用 dsn.Resolver 拿源端 *gorm.DB（占位 StaticResolver 模式下查的是控制库，
//     真生产 Resolver 按 sourceDsnRef → Vault → 源库 DSN 建连）
//   - binlog_format / gtid_mode：SHOW VARIABLES
//   - 表 PK：SHOW INDEX FROM <table> WHERE Key_name='PRIMARY'
//   - ready：上述三项全通过才 true；任一不通过仍返 200 但 ready=false，运维侧决定是否继续
//
// 不查 read_replica_lag（需 SHOW REPLICA STATUS，权限/版本兼容性问题大），恒返 0。
func (h *InternalTaskHandler) Preflight(ctx *gin.Context, req preflightReq) (Result, error) {
	if h.resolver == nil {
		return Result{}, errEngineNotConfigured
	}
	// 用 fake task 借用 Resolver 拿 src db（占位 Resolver 不看 task 字段；
	// 真生产 Resolver 按 req.SourceDsnRef 解 DSN，此处用 fake task 携带）
	fakeTask := domain.Task{SourceDsnRef: req.SourceDsnRef}
	db, err := h.resolver.ResolveSrc(ctx.Request.Context(), fakeTask)
	if err != nil {
		return Result{}, errs.New(503, "resolve source db failed").WithCause(err)
	}

	binlogFormat := queryVariable(ctx.Request.Context(), db, "binlog_format")
	gtidMode := queryVariable(ctx.Request.Context(), db, "gtid_mode")
	tablesWithPK := make([]string, 0, len(req.Tables))
	tablesMissingPK := make([]string, 0)
	for _, t := range req.Tables {
		if hasPrimaryKey(ctx.Request.Context(), db, t) {
			tablesWithPK = append(tablesWithPK, t)
		} else {
			tablesMissingPK = append(tablesMissingPK, t)
		}
	}
	// ready 判断只看当前 migrator 真依赖的能力:
	//   - binlog_format=ROW(Canal 解析 RowsEvent 必需)
	//   - 每张表有 PK(全量 PK 范围切片 + 增量 dedup 必需)
	// gtid_mode 是信息性字段:返回供运维感知,但 ready 不强制要求 ON。
	// 当前 BinlogClient 用 file/pos 续订,不依赖 server 端 gtid_mode。
	ready := strings.EqualFold(binlogFormat, "ROW") && len(tablesMissingPK) == 0

	data := gin.H{
		"ready":             ready,
		"binlog_format":     binlogFormat,
		"gtid_mode":         gtidMode,
		"tables_with_pk":    tablesWithPK,
		"read_replica_lag":  0, // 不查 SHOW REPLICA STATUS（权限/版本兼容性差），恒返 0
		"tables_missing_pk": tablesMissingPK,
	}
	return Result{Data: data}, nil
}

// queryVariable 查 MySQL 系统变量。
//
// 实现细节：SHOW VARIABLES LIKE 'xxx' 的列名是 `Variable_name` / `Value`，
// GORM 默认 NamingStrategy 把 struct `VariableName` 转 `variable_name`，跟 MySQL 不匹配 → 返空。
// 改用 `SELECT @@global.<name>` 直接拿单值，绕开列名映射；name 走白名单防 SQL 注入。
func queryVariable(ctx context.Context, db *gorm.DB, name string) string {
	switch name {
	case "binlog_format", "gtid_mode":
		// 白名单允许
	default:
		return ""
	}
	var v string
	if err := db.WithContext(ctx).
		Raw("SELECT @@global." + name).
		Scan(&v).Error; err != nil {
		return ""
	}
	return v
}

// hasPrimaryKey 通过 information_schema 查表是否有 PRIMARY KEY（兼容性比 SHOW INDEX 更好）。
func hasPrimaryKey(ctx context.Context, db *gorm.DB, table string) bool {
	var n int64
	if err := db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM information_schema.STATISTICS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = 'PRIMARY'`,
		table,
	).Scan(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// ── /tasks/:id/start ───────────────────────────────────────
type startReq struct {
	Phase  string       `json:"phase" binding:"required,oneof=full incr"`
	Shards []shardInput `json:"shards,omitempty"`
}

type shardInput struct {
	No      int   `json:"no"`
	PKMin   int64 `json:"pkMin"`
	PKMax   int64 `json:"pkMax"`
	BatchSz int   `json:"batchSz,omitempty"`
}

// Start 异步启动 FullEngine 或 IncrEngine（HTTP 请求立即返回）。
// 错误只 log；task.status 持久化 + 失败回写见已知边界扩展点。
//
// shards 自动切片：当请求体 shards 为空且 srcSource 实现 PKRanger 时，
// handler 在启动 goroutine 前先 SELECT MIN/MAX 拿 PK 范围 + PlanShards 自动切；
// 这样调用方不需要事先知道源表的 PK 上下界。
func (h *InternalTaskHandler) Start(ctx *gin.Context, req startReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	switch req.Phase {
	case "full":
		if h.fullEng == nil {
			return Result{}, errEngineNotConfigured
		}
		if h.fullEng.IsRunning(id) {
			return Result{}, migratorerrs.ErrTaskAlreadyRunning
		}
		shards, sErr := h.resolveShards(ctx.Request.Context(), id, req.Shards)
		if sErr != nil {
			return Result{}, sErr
		}
		// 应用 throttle 配置（Throttle endpoint 写的 cache），覆盖每个 shard 的 QPS
		if cfg := h.readThrottle(ctx.Request.Context(), id); cfg.QPS > 0 {
			for i := range shards {
				shards[i].QPSLimit = cfg.QPS
			}
		}
		go func() {
			// engine 内部 LoadOrStore 防 race；handler 的 IsRunning 仅做友好响应，
			// 真 race 守门员在 engine.Run 里（返 ErrTaskAlreadyRunning 时只 log Debug 不 Warn）。
			if err := h.fullEng.Run(detachCtx(ctx), id, shards); err != nil {
				if errors.Is(err, migratorerrs.ErrTaskAlreadyRunning) {
					return
				}
				h.l.Warn("FullEngine.Run failed", logger.Int64("task_id", id), logger.Error(err))
			}
		}()
	case "incr":
		if h.incrEng == nil {
			return Result{}, errEngineNotConfigured
		}
		if h.incrEng.IsRunning(id) {
			return Result{}, migratorerrs.ErrTaskAlreadyRunning
		}
		go func() {
			if err := h.incrEng.Run(detachCtx(ctx), id); err != nil {
				if errors.Is(err, migratorerrs.ErrTaskAlreadyRunning) {
					return
				}
				h.l.Warn("IncrEngine.Run failed", logger.Int64("task_id", id), logger.Error(err))
			}
		}()
	}
	return Result{Data: gin.H{"started": req.Phase}}, nil
}

// ── /tasks/:id/pause ───────────────────────────────────────
func (h *InternalTaskHandler) Pause(ctx *gin.Context) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.fullEng == nil && h.incrEng == nil {
		return Result{}, errEngineNotConfigured
	}
	var fullErr, incrErr error
	if h.fullEng != nil {
		fullErr = h.fullEng.Pause(id)
	} else {
		fullErr = errEngineNotConfigured
	}
	if h.incrEng != nil {
		incrErr = h.incrEng.Pause(id)
	} else {
		incrErr = errEngineNotConfigured
	}
	if fullErr != nil && incrErr != nil {
		return Result{}, errs.New(409, "task not running (no full / incr engine paused)")
	}
	return Result{Data: gin.H{"paused": true}}, nil
}

// ── /tasks/:id/throttle ────────────────────────────────────
type throttleReq struct {
	QPS          int `json:"qps,omitempty"`
	ShardWorkers int `json:"shard_workers,omitempty"`
}

// Throttle 写限速配置（TaskService 持久化）。下次 Start 时 resolveShards 回读覆盖 ShardSpec。
//
// 限速"下次 Start 生效"（非实时）：源库压力大时 → 改 throttle → pause → 重启。
//
// payload 例：{"qps": 5000, "shard_workers": 8}；qps <= 0 && workers <= 0 → 清空配置（恢复默认）。
func (h *InternalTaskHandler) Throttle(ctx *gin.Context, req throttleReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	cfg := domain.ThrottleConfig{QPS: req.QPS, ShardWorkers: req.ShardWorkers}
	if cfg.Empty() {
		if clrErr := h.svc.ClearThrottle(ctx.Request.Context(), id); clrErr != nil {
			return Result{}, clrErr
		}
		return Result{Data: gin.H{"cleared": true}}, nil
	}
	if setErr := h.svc.SetThrottle(ctx.Request.Context(), id, cfg); setErr != nil {
		return Result{}, setErr
	}
	return Result{Data: gin.H{
		"qps":           cfg.QPS,
		"shard_workers": cfg.ShardWorkers,
		"applied_on":    "next_start",
	}}, nil
}

// readThrottle 读限速配置；不存在或故障 → 返 zero config（用默认）。
func (h *InternalTaskHandler) readThrottle(ctx context.Context, taskId int64) domain.ThrottleConfig {
	cfg, ok, err := h.svc.GetThrottle(ctx, taskId)
	if err != nil {
		h.l.Warn("throttle read failed; falling back to defaults",
			logger.Int64("task_id", taskId), logger.Error(err))
		return domain.ThrottleConfig{}
	}
	if !ok {
		return domain.ThrottleConfig{}
	}
	return cfg
}

// ── /tasks/:id/gray ────────────────────────────────────────
type grayReq struct {
	// Percent 不在 binding 限制范围（min/max）—— 把越界值透传到 SwitchService.SetGray，
	// 由 ErrInvalidGrayPercent 返业务消息 "灰度比例必须在 0-100 之间"，避免被 ginx
	// 通用的 "参数错误" 吞掉，前端定位更精确。
	Percent int `json:"percent"`
}

func (h *InternalTaskHandler) SetGray(ctx *gin.Context, req grayReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.swSvc == nil {
		return Result{}, errEngineNotConfigured
	}
	if err := h.swSvc.SetGray(ctx.Request.Context(), id, req.Percent); err != nil {
		return Result{}, err
	}
	return Result{Data: gin.H{"gray": req.Percent}}, nil
}

// ── /tasks/:id/switch ──────────────────────────────────────
type switchReq struct {
	// Stage 推进目标阶段。rollback 模式下忽略（可传 "" 或 "ignored"）；
	// 推进模式下由 handler 校验 stage.Valid()，不在 binding 限制（避免 rollback 时被 required 误拦）。
	Stage   string `json:"stage,omitempty"`
	Action  string `json:"action,omitempty"` // propose / approve / rollback
	Propose string `json:"propose,omitempty"`
	Approve string `json:"approve,omitempty"`
}

func (h *InternalTaskHandler) SetSwitch(ctx *gin.Context, req switchReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.swSvc == nil {
		return Result{}, errEngineNotConfigured
	}
	if req.Action == "rollback" {
		if err := h.swSvc.Rollback(ctx.Request.Context(), id); err != nil {
			return Result{}, err
		}
		return Result{Data: gin.H{"stage": domain.StageSrcFirst}}, nil
	}
	stage := domain.Stage(req.Stage)
	if !stage.Valid() {
		return Result{}, errs.New(400, "invalid stage").WithMetadata("stage", req.Stage)
	}
	if err := h.swSvc.SetStage(ctx.Request.Context(), id, stage, req.Propose, req.Approve); err != nil {
		return Result{}, err
	}
	return Result{Data: gin.H{"stage": stage}}, nil
}

// ── /tasks/:id/lag ─────────────────────────────────────────
//
// 返回双侧延迟：
//
//	srcLagMs  源端 binlog 最新事件 → 现在；反映消费跟上没有
//	dstLagMs  最近一次 Sink.Apply 成功事件的 EventTs → 现在；反映写到对端有多新
//	lagMs     = srcLagMs（别名）
//
// 任一侧不可用（Source 未实现 LagReporter / 尚无 Apply 完成）返 -1。
func (h *InternalTaskHandler) Lag(ctx *gin.Context) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.incrEng == nil {
		return Result{}, errEngineNotConfigured
	}
	srcLag, srcErr := h.incrEng.Lag(id)
	dstLag, dstErr := h.incrEng.LagDst(id)
	if srcErr != nil && dstErr != nil {
		// 两侧都拿不到 → 视为任务未跑（503 与旧行为一致）
		return Result{}, errs.New(503, "lag unavailable").WithCause(srcErr)
	}
	if srcErr != nil {
		srcLag = -1
	}
	if dstErr != nil {
		dstLag = -1
	}
	return Result{Data: gin.H{
		"lagMs":    srcLag,
		"srcLagMs": srcLag,
		"dstLagMs": dstLag,
	}}, nil
}

// ── /tasks/:id/verify ──────────────────────────────────────
type verifyReq struct {
	Mode string `json:"mode" binding:"required,oneof=sample full"`
	// SampleRate 用指针区分"未传字段"和"显式传 0"：
	//   nil   → 未传，sample mode 默认 0.01
	//   非 nil → 必须 (0, 1]，否则返 ErrInvalidSampleRate
	SampleRate *float64 `json:"sampleRate,omitempty"`
}

func (h *InternalTaskHandler) Verify(ctx *gin.Context, req verifyReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.verEng == nil {
		return Result{}, errEngineNotConfigured
	}
	var mismatch int64
	switch req.Mode {
	case "full":
		mismatch, err = h.verEng.Full(ctx.Request.Context(), id)
	case "sample":
		rate := 0.01 // 默认 1%
		if req.SampleRate != nil {
			if *req.SampleRate <= 0 || *req.SampleRate > 1 {
				return Result{}, migratorerrs.ErrInvalidSampleRate
			}
			rate = *req.SampleRate
		}
		mismatch, err = h.verEng.Sample(ctx.Request.Context(), id, rate)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Data: gin.H{"mismatchCount": mismatch}}, nil
}

// ── /tasks/:id/mismatch ────────────────────────────────────

// MismatchVO 对账差异条目对外响应（camelCase 命名，与 webook 其他 API 统一）。
type MismatchVO struct {
	Id           int64  `json:"id"`
	TaskId       int64  `json:"taskId"`
	Direction    string `json:"direction"`
	BizTable     string `json:"bizTable"`
	BizId        string `json:"bizId"`
	MismatchKind string `json:"mismatchKind"`
	DiffDetail   string `json:"diffDetail"`
	Repaired     int8   `json:"repaired"`
	CreatedAt    int64  `json:"createdAt"`
	RepairedAt   int64  `json:"repairedAt"`
}

func toMismatchVO(d domain.ValidateLog) MismatchVO {
	return MismatchVO{
		Id: d.Id, TaskId: d.TaskId, Direction: d.Direction,
		BizTable: d.BizTable, BizId: d.BizId,
		MismatchKind: d.MismatchKind, DiffDetail: d.DiffDetail,
		Repaired: d.Repaired, CreatedAt: d.CreatedAt, RepairedAt: d.RepairedAt,
	}
}

func (h *InternalTaskHandler) Mismatch(ctx *gin.Context) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.verEng == nil {
		return Result{}, errEngineNotConfigured
	}
	offset, oerr := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	if oerr != nil {
		offset = 0
	}
	limit, lerr := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	if lerr != nil || limit <= 0 || limit > 200 {
		limit = 50
	}
	list, total, err := h.verEng.ListMismatch(ctx.Request.Context(), id, offset, limit)
	if err != nil {
		return Result{}, err
	}
	vos := make([]MismatchVO, 0, len(list))
	for _, d := range list {
		vos = append(vos, toMismatchVO(d))
	}
	return Result{Data: ginx.PageResult{List: vos, Total: total}}, nil
}

// ── /tasks/:id/repair ──────────────────────────────────────
type repairReq struct {
	Strategy string  `json:"strategy" binding:"required,oneof=src_overwrite_dst dst_overwrite_src mark_only"`
	IDs      []int64 `json:"ids" binding:"required,min=1"`
}

func (h *InternalTaskHandler) Repair(ctx *gin.Context, req repairReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.verEng == nil {
		return Result{}, errEngineNotConfigured
	}
	count, err := h.verEng.Repair(ctx.Request.Context(), id, verify.RepairStrategy(req.Strategy), req.IDs)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: gin.H{"repaired": count}}, nil
}

// ── /tasks/:id/replay-dl ───────────────────────────────────
type replayReq struct {
	Limit int `json:"limit,omitempty"`
}

// ReplayDL 重放死信队列（业务在 replay.ReplayService：list → Sink.Apply → MarkReplayed，失败 IncrementRetry 累计）。
func (h *InternalTaskHandler) ReplayDL(ctx *gin.Context, req replayReq) (Result, error) {
	id, err := taskIdFromPath(ctx)
	if err != nil {
		return Result{}, err
	}
	if h.replaySvc == nil {
		return Result{}, errEngineNotConfigured
	}
	replayed, failed, err := h.replaySvc.ReplayDeadLetters(ctx.Request.Context(), id, req.Limit)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: gin.H{"replayed": replayed, "failed": failed}}, nil
}

// ═════════════════════════════════════════════════════════════
// helpers
// ═════════════════════════════════════════════════════════════

// buildShards 把 API 入参 shardInput[] 转 source.ShardSpec[]。
// 调用方提供 shards 时直接转换；空输入由 resolveShards 决定回退策略。
func buildShards(in []shardInput) []source.ShardSpec {
	shards := make([]source.ShardSpec, len(in))
	for i, s := range in {
		shards[i] = source.ShardSpec{
			No: s.No, PKMin: s.PKMin, PKMax: s.PKMax, BatchSz: s.BatchSz,
		}
	}
	return shards
}

// resolveShards 决定 FullEngine.Run 用的 ShardSpec[]：
//
//  1. req.Shards 非空 → 直接转换
//  2. factory build src 实现 PKRanger → SELECT MIN/MAX 后用 PlanShards 自动切（默认 16 片）
//  3. 兜底 → 单分片 [1, 1<<62]
func (h *InternalTaskHandler) resolveShards(ctx context.Context, taskId int64, reqShards []shardInput) ([]source.ShardSpec, error) {
	if len(reqShards) > 0 {
		return buildShards(reqShards), nil
	}
	if h.srcFactory == nil {
		return []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 1 << 62}}, nil
	}
	task, err := h.svc.Get(ctx, taskId)
	if err != nil {
		return nil, err
	}
	// resolveShards 用于 FullEngine 启动时为单表 task 提供 hint shards；
	// 多表 task 时 FullEngine 自己按 table 走 PKRange，handler 不传 hint。
	tables, err := task.Tables()
	if err != nil {
		return nil, err
	}
	if len(tables) > 1 {
		// 多表 task：handler 不提供 hint shards，引擎按表自动切片
		return nil, nil
	}
	src, err := h.srcFactory.BuildFullSrc(ctx, task, 0)
	if err != nil {
		return nil, err
	}
	ranger, ok := src.(source.PKRanger)
	if !ok {
		return []source.ShardSpec{{No: 0, PKMin: 1, PKMax: 1 << 62}}, nil
	}
	minPK, maxPK, err := ranger.PKRange(ctx)
	if err != nil {
		return nil, errs.New(503, "PKRange query failed").WithCause(err)
	}
	if minPK == 0 && maxPK == 0 {
		// 空表 → 单分片 0..0（FullScan 立即返 nil，不报错）
		return []source.ShardSpec{{No: 0, PKMin: 0, PKMax: 0}}, nil
	}
	return full.PlanShards(minPK, maxPK, defaultShardCount), nil
}

// defaultShardCount 自动切片时的默认分片数。
const defaultShardCount = 16

// detachCtx 把请求 ctx 换成 context.Background()，避免 HTTP 请求结束后 ctx cancel 杀掉异步引擎。
func detachCtx(_ *gin.Context) context.Context {
	return context.Background()
}

// ═════════════════════════════════════════════════════════════
// VO（视图对象）
// ═════════════════════════════════════════════════════════════

// TaskVO 视图对象，屏蔽 domain.Task 中的 secret reference（SourceDsnRef/SinkDsnRef/TablesJSON）。
type TaskVO struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	Kind        string `json:"kind"`
	SourceType  string `json:"sourceType"`
	SinkType    string `json:"sinkType"`
	Status      int8   `json:"status"`
	GrayPercent int16  `json:"grayPercent"`
	Consistency string `json:"consistency"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

func toTaskVO(t domain.Task) TaskVO {
	return TaskVO{
		Id:          t.Id,
		Name:        t.Name,
		Mode:        string(t.Mode),
		Kind:        string(t.Kind),
		SourceType:  string(t.SourceType),
		SinkType:    t.SinkType,
		Status:      int8(t.Status),
		GrayPercent: t.GrayPercent,
		Consistency: t.Consistency,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}
