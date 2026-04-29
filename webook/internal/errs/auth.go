package errs

import "github.com/webook/pkg/errs"

// 用户注册 / 登录相关错误。HTTP code 选择：
//
//	400 InvalidArgument — 参数校验失败（格式/长度/不一致）
//	401 Unauthenticated — 凭证错误
//	409 Conflict        — 资源冲突（重复注册）
var (
	ErrDuplicateUser         = errs.New(409, "此用户已被注册")
	ErrDuplicateEmail        = errs.New(409, "邮箱已被注册")
	ErrInvalidUserOrPassword = errs.New(401, "用户或密码错误")
)

// 用户参数校验错误（注册/登录链路）
var (
	ErrInvalidEmailFormat = errs.New(400, "非法邮箱格式")
	ErrPasswordMismatch   = errs.New(400, "两次输入密码不匹配")
	ErrPasswordWeak       = errs.New(400, "密码必须包含字母、数字、特殊字符，并且不少于八位")
	ErrPhoneEmpty         = errs.New(400, "请输入手机号码")
	ErrSMSCodeWrong       = errs.New(400, "验证码错误，请重新输入")
)

// OAuth2 微信登录相关
var (
	ErrWechatStateInvalid = errs.New(400, "非法请求")
	ErrWechatCodeInvalid  = errs.New(400, "授权码有误")
)
