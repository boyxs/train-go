package ginx

// Result 统一 HTTP 响应格式
//
// Code 约定（项目级）：
//
//	0 = 成功
//	4 = 客户端错误（参数/权限/业务校验等）
//	5 = 服务端错误（系统异常）
type Result struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}
