package errs

import "github.com/webook/pkg/errs"

// 文章作者侧错误（DAO 层用 author_id WHERE 条件做权限收敛，找不到统一报 NotFound）
var (
	ErrArticleNotFound            = errs.New(404, "文章不存在或无权限")
	ErrArticleEmptyTitleOrContent = errs.New(400, "标题和内容不能为空")
)

// 文章搜索（ES）相关错误
var (
	ErrSearchDocNotFound = errs.New(404, "搜索文档不存在")
	ErrESDocNotFound     = errs.New(404, "ES 文档不存在")
	ErrSearchQueryEmpty  = errs.New(400, "搜索内容不能为空")
)

// 文章润色业务校验错误（400/429）
var (
	ErrPolishEmptyTitle     = errs.New(400, "标题不能为空")
	ErrPolishEmptyContent   = errs.New(400, "内容不能为空")
	ErrPolishContentTooLong = errs.New(400, "内容过长，请缩减至 10000 字符以内")
	ErrPolishRateLimit      = errs.New(429, "润色次数已达上限，请稍后再试")
)

// 文章榜单 / 互动相关
var (
	ErrInvalidDimension = errs.New(400, "无效的榜单维度")
	ErrInvalidDate      = errs.New(400, "无效的日期")
	ErrInvalidRank      = errs.New(400, "无效的名次")
	ErrClickInvalidArgs = errs.New(400, "参数无效")
)
