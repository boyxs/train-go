package ginx

// PageResult 统一分页响应，与前端 types.PageResult<T> 对齐。
//
// 用法：
//
//	return Result{Data: PageResult{List: list, Total: total}}, nil
type PageResult struct {
	List  any   `json:"list"`
	Total int64 `json:"total"`
}
