package errs

import "github.com/boyxs/train-go/webook/pkg/errs"

// 搜索业务错误 sentinel；Code 即 HTTP status，Reason 业务原因码。
var (
	ErrSearchQueryEmpty   = errs.New(400, "搜索内容不能为空").WithReason("SEARCH_QUERY_EMPTY")
	ErrSearchQueryTooLong = errs.New(400, "搜索内容过长").WithReason("SEARCH_QUERY_TOO_LONG")
	ErrSearchDocNotFound  = errs.New(404, "搜索文档不存在").WithReason("SEARCH_DOC_NOT_FOUND")
)
