package errs

import "github.com/webook/pkg/errs"

// 关系业务错误 sentinel；Code 即 HTTP status，Reason 业务原因码，由 errconv 拦截器统一转 gRPC status。
var (
	ErrFollowSelf      = errs.New(400, "不能关注自己").WithReason("RELATION_FOLLOW_SELF")
	ErrBlockSelf       = errs.New(400, "不能拉黑自己").WithReason("RELATION_BLOCK_SELF")
	ErrBlockedTarget   = errs.New(409, "你已拉黑对方，请先取消拉黑").WithReason("RELATION_BLOCKED_TARGET")
	ErrBlockedByTarget = errs.New(409, "对方已将你拉黑，无法关注").WithReason("RELATION_BLOCKED_BY_TARGET")
	ErrInvalidArg      = errs.New(400, "参数不合法").WithReason("RELATION_ARGUMENT_INVALID")
)
