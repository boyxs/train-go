package errs

import "github.com/webook/pkg/errs"

// 评论业务错误 sentinel；Code 即 HTTP status（pkg/errs 规范）。
var (
	ErrContentEmpty     = errs.New(400, "评论内容不能为空")
	ErrContentTooLong   = errs.New(400, "评论内容不能超过 500 字")
	ErrSensitiveContent = errs.New(422, "内容包含敏感词，请修改后再发布")
	ErrRateLimited      = errs.New(429, "操作太频繁，请稍后再试")
	ErrForbidden        = errs.New(403, "只能删除自己的评论")
)
