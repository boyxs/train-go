// Package errs 是 webook-migrator 服务的业务 sentinel 错误集合。
//
// 设计风格（与 webook/internal/errs / webook/chat/errs 一致）：
//   - 用 pkg/errs.New(httpCode, message).WithReason(reason) 构造 *errs.Error
//   - Reason 为业务原因码（SCREAMING_SNAKE，全局唯一），errors.Is 优先按 Reason 比对
//   - Handler 直接 return *errs.Error，ginx.Wrap 自动把 Code 翻译成 HTTP status + 带出 reason
package errs

import "github.com/webook/pkg/errs"

var (
	// 400 参数 / 校验类
	ErrInvalidArgument      = errs.New(400, "参数不合法").WithReason("MIGRATOR_INVALID_ARGUMENT")
	ErrInvalidGrayPercent   = errs.New(400, "灰度比例必须在 0-100 之间").WithReason("MIGRATOR_GRAY_PERCENT_INVALID")
	ErrInvalidSampleRate    = errs.New(400, "采样率必须在 (0, 1] 之间").WithReason("MIGRATOR_SAMPLE_RATE_INVALID")
	ErrProposeActorRequired = errs.New(400, "DST_ONLY 切流需要 propose 发起人").WithReason("MIGRATOR_PROPOSE_ACTOR_REQUIRED")

	// 404 资源不存在
	ErrTaskNotFound = errs.New(404, "迁移任务不存在").WithReason("MIGRATOR_TASK_NOT_FOUND")

	// 409 状态冲突
	ErrDuplicateTaskName         = errs.New(409, "迁移任务名已存在").WithReason("MIGRATOR_TASK_NAME_DUPLICATE")
	ErrInvalidStatusTransition   = errs.New(409, "状态机非法转移").WithReason("MIGRATOR_STATUS_TRANSITION_INVALID")
	ErrCheckpointVersionConflict = errs.New(409, "checkpoint 乐观锁冲突，其他 worker 已更新").WithReason("MIGRATOR_CHECKPOINT_CONFLICT")
	ErrTaskAlreadyRunning        = errs.New(409, "任务已在运行").WithReason("MIGRATOR_TASK_ALREADY_RUNNING")
	ErrRollbackNotAllowed        = errs.New(409, "回滚仅双写期（SRC_FIRST/DST_FIRST）可用").WithReason("MIGRATOR_ROLLBACK_NOT_ALLOWED")

	// 501 能力未装配
	ErrThrottleNotConfigured = errs.New(501, "throttle 存储未配置（wire 未注入 ThrottleRepository）").WithReason("MIGRATOR_THROTTLE_NOT_CONFIGURED")
)
