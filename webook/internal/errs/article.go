package errs

import "github.com/webook/pkg/errs"

// 文章作者侧错误（DAO 层用 author_id WHERE 条件做权限收敛，找不到统一报 NotFound）
var (
	ErrArticleNotFound            = errs.New(404, "文章不存在或无权限").WithReason("ARTICLE_NOT_FOUND")
	ErrArticleEmptyTitleOrContent = errs.New(400, "标题和内容不能为空").WithReason("ARTICLE_TITLE_OR_CONTENT_EMPTY")
)

// 文章搜索（ES）相关错误
var (
	ErrSearchDocNotFound = errs.New(404, "搜索文档不存在").WithReason("SEARCH_DOC_NOT_FOUND")
	ErrESDocNotFound     = errs.New(404, "ES 文档不存在").WithReason("SEARCH_ES_DOC_NOT_FOUND")
	ErrSearchQueryEmpty  = errs.New(400, "搜索内容不能为空").WithReason("SEARCH_QUERY_EMPTY")
)

// 文章润色业务校验错误（400/429）
var (
	ErrPolishEmptyTitle     = errs.New(400, "标题不能为空").WithReason("POLISH_TITLE_EMPTY")
	ErrPolishEmptyContent   = errs.New(400, "内容不能为空").WithReason("POLISH_CONTENT_EMPTY")
	ErrPolishContentTooLong = errs.New(400, "内容过长，请缩减至 10000 字符以内").WithReason("POLISH_CONTENT_TOO_LONG")
	ErrPolishRateLimit      = errs.New(429, "润色次数已达上限，请稍后再试").WithReason("POLISH_RATE_LIMITED")
)

// 文章榜单 / 互动相关
var (
	ErrInvalidDimension = errs.New(400, "无效的榜单维度").WithReason("RANKING_DIMENSION_INVALID")
	ErrInvalidDate      = errs.New(400, "无效的日期").WithReason("RANKING_DATE_INVALID")
	ErrInvalidRank      = errs.New(400, "无效的名次").WithReason("RANKING_RANK_INVALID")
	ErrClickInvalidArgs = errs.New(400, "参数无效").WithReason("CLICK_INVALID_ARGUMENT")
)
