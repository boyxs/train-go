package errs

import "github.com/webook/pkg/errs"

// 短信验证码相关错误。HTTP code 选择：
//
//	400 — 验证码失效（用户输入错或过期，业务校验失败）
//	429 — 触发限流（发送/校验过于频繁）
var (
	ErrCodeInvalid       = errs.New(400, "验证码错误或已过期")
	ErrCodeSendTooMany   = errs.New(429, "验证码发送太频繁")
	ErrCodeVerifyTooMany = errs.New(429, "验证码校验太频繁")
)
