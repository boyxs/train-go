// Package errs 集中定义 chat 服务跨层共享的业务 sentinel，与 chat/consts 平行。
// Error 类型本身在 pkg/errs（项目通用），本包仅定义业务错误变量。
package errs

import (
	"gorm.io/gorm"

	"github.com/webook/pkg/errs"
)

// ErrRecordNotFound 通用「记录不存在」，alias gorm.ErrRecordNotFound
// 让 errors.Is(err, errs.ErrRecordNotFound) 直通底层。
var ErrRecordNotFound = gorm.ErrRecordNotFound

// 业务错误：会话 / 消息相关
//
//	404 — 会话不存在或越权访问
//	400 — 请求体校验失败
//	429 — 触发限流
var (
	ErrConversationNotFound = errs.New(404, "对话不存在")
	ErrMessageTooLong       = errs.New(400, "消息内容过长")
	ErrFeedbackInvalid      = errs.New(400, "无效的反馈值")
	ErrChatInvalidArgs      = errs.New(400, "参数错误")
	ErrChatRateLimit        = errs.New(429, "发送过于频繁，请稍后再试")
)
