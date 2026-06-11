// Package errs 是 webook-migrator 服务的业务 sentinel 错误集合。
//
// 设计风格（与 webook/internal/errs / webook/chat/errs 一致）：
//   - 用 pkg/errs.New(httpCode, message) 构造 *errs.Error
//   - Message 必须全局唯一（errors.Is 按 Code+Message 比对）
//   - Handler 直接 return *errs.Error，ginx.Wrap 自动把 Code 翻译成 HTTP status
package errs

import "github.com/webook/pkg/errs"

var (
	// 400 参数 / 校验类
	ErrInvalidArgument      = errs.New(400, "参数不合法")
	ErrInvalidGrayPercent   = errs.New(400, "灰度比例必须在 0-100 之间")
	ErrInvalidSampleRate    = errs.New(400, "采样率必须在 (0, 1] 之间")
	ErrProposeActorRequired = errs.New(400, "DST_ONLY 切流需要 propose 发起人")

	// 404 资源不存在
	ErrTaskNotFound = errs.New(404, "迁移任务不存在")

	// 409 状态冲突
	ErrDuplicateTaskName         = errs.New(409, "迁移任务名已存在")
	ErrInvalidStatusTransition   = errs.New(409, "状态机非法转移")
	ErrCheckpointVersionConflict = errs.New(409, "checkpoint 乐观锁冲突，其他 worker 已更新")
	ErrTaskAlreadyRunning        = errs.New(409, "任务已在运行")
	ErrRollbackNotAllowed        = errs.New(409, "回滚仅双写期（SRC_FIRST/DST_FIRST）可用")

	// 501 能力未装配
	ErrThrottleNotConfigured = errs.New(501, "throttle 存储未配置（wire 未注入 ThrottleRepository）")
)
