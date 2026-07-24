package errs

import "github.com/boyxs/train-go/webook/pkg/errs"

// feed 业务错误 sentinel；Code 即 HTTP status，Reason 业务原因码，由 errconv 拦截器统一转 gRPC status。
var (
	ErrInvalidArg = errs.New(400, "参数不合法").WithReason("FEED_ARGUMENT_INVALID")
)
