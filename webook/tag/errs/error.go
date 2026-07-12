package errs

import "github.com/boyxs/train-go/webook/pkg/errs"

// 标签业务错误 sentinel；Code 即 HTTP status，Reason 业务原因码，由 errconv 拦截器统一转 gRPC status。
var (
	ErrTagLimitExceeded = errs.New(400, "每篇最多 5 个标签").WithReason("TAG_LIMIT_EXCEEDED")
	ErrTagNameInvalid   = errs.New(400, "标签名不合法（1–30 字，不含 /?#）").WithReason("TAG_NAME_INVALID")
	ErrTagNotFound      = errs.New(404, "标签不存在").WithReason("TAG_NOT_FOUND")
)
