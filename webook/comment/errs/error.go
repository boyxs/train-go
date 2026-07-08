package errs

import "github.com/boyxs/train-go/webook/pkg/errs"

var (
	ErrContentEmpty     = errs.New(400, "评论内容不能为空").WithReason("COMMENT_CONTENT_EMPTY")
	ErrContentTooLong   = errs.New(400, "评论内容不能超过 500 字").WithReason("COMMENT_CONTENT_TOO_LONG")
	ErrSensitiveContent = errs.New(422, "内容包含敏感词，请修改后再发布").WithReason("COMMENT_CONTENT_SENSITIVE")
	ErrRateLimited      = errs.New(429, "操作太频繁，请稍后再试").WithReason("COMMENT_RATE_LIMITED")
	ErrForbidden        = errs.New(403, "只能删除自己的评论").WithReason("COMMENT_FORBIDDEN")
)
