package ginx

// Result 统一 HTTP 响应格式：Code ≡ HTTP status；Reason/Metadata 仅 *errs.Error 路径有值。
type Result struct {
	Code     int               `json:"code"`
	Reason   string            `json:"reason,omitempty"`
	Msg      string            `json:"msg"`
	Data     any               `json:"data"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func Success(data any) Result           { return Result{Code: 200, Msg: "OK", Data: data} }
func BadRequest(msg string) Result      { return Result{Code: 400, Msg: msg} }
func Unauthorized(msg string) Result    { return Result{Code: 401, Msg: msg} }
func Forbidden(msg string) Result       { return Result{Code: 403, Msg: msg} }
func NotFound(msg string) Result        { return Result{Code: 404, Msg: msg} }
func Conflict(msg string) Result        { return Result{Code: 409, Msg: msg} }
func TooManyRequests(msg string) Result { return Result{Code: 429, Msg: msg} }
func Internal(msg string) Result        { return Result{Code: 500, Msg: msg} }
