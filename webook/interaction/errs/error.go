package errs

import "github.com/boyxs/train-go/webook/pkg/errs"

// 互动业务错误 sentinel；Code 即 HTTP status，Reason 业务原因码，由 errconv 拦截器统一转 gRPC status。
var (
	ErrUnauthenticated = errs.New(401, "请先登录").WithReason("INTERACTION_UNAUTHENTICATED")
	ErrBizEmpty        = errs.New(400, "biz 不能为空").WithReason("INTERACTION_BIZ_EMPTY")
	ErrBizIdEmpty      = errs.New(400, "bizId 不能为空").WithReason("INTERACTION_BIZ_ID_EMPTY")
)
