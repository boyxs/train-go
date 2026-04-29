package ginx

// Result 统一 HTTP 响应格式。
//
//	Code     直接用 HTTP status code（200/400/401/403/404/409/429/500/...）
//	Msg      人类可读消息（中文友好提示）
//	Data     成功路径的响应数据
//	Metadata 业务错误的附加上下文（resourceID / field 等），仅 *errs.Error 路径 populated
//
// 一致性约定：
//
//	HTTP status code = body.Code（前端无需双重判断；axios 拦截器看 HTTP status 触发 catch）
//	业务错误抛 *errs.Error（sentinel 在 internal/errs 集中定义），ginx.Wrap 自动 build Result
//	handler 仅写业务逻辑，return ginx.Result{Data}, err — 不写 Code 字面量、不写 switch
type Result struct {
	Code     int               `json:"code"`
	Msg      string            `json:"msg"`
	Data     any               `json:"data"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
