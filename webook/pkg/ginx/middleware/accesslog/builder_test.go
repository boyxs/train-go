package accesslog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"maxLen=0 不截断", "hello", 0, "hello"},
		{"maxLen<0 不截断", "hello", -1, "hello"},
		{"len<maxLen 原样", "hello", 10, "hello"},
		{"len==maxLen 原样", "hello", 5, "hello"},
		{"len>maxLen 截断", "hello", 3, "hel"},
		{"空串 maxLen>0", "", 5, ""},
		{"空串 maxLen=0", "", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := truncate(c.in, c.maxLen); got != c.want {
				t.Fatalf("truncate(%q,%d)=%q want %q", c.in, c.maxLen, got, c.want)
			}
		})
	}
}

func TestNewLoggerMiddlewareBuilder_defaults(t *testing.T) {
	b := NewLoggerMiddlewareBuilder(func(context.Context, RequestLog) {})
	if b == nil {
		t.Fatal("builder 不应为 nil")
	}
	if b.allowReqBody.Load() || b.allowResBody.Load() {
		t.Fatal("默认 allowReqBody/allowResBody 应为 false")
	}
	if b.maxReqLen.Load() != 0 || b.maxResLen.Load() != 0 || b.maxPathLen.Load() != 0 {
		t.Fatal("默认 max*Len 应为 0")
	}
}

func TestAllowBody_chainableAndStore(t *testing.T) {
	b := NewLoggerMiddlewareBuilder(func(context.Context, RequestLog) {})
	if got := b.AllowReqBody(true).AllowResBody(true); got != b {
		t.Fatal("setter 应返回自身以支持链式")
	}
	if !b.allowReqBody.Load() || !b.allowResBody.Load() {
		t.Fatal("AllowReqBody/AllowResBody(true) 应置 true")
	}
	b.AllowReqBody(false).AllowResBody(false)
	if b.allowReqBody.Load() || b.allowResBody.Load() {
		t.Fatal("置 false 应生效")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("成功_覆盖全部字段", func(t *testing.T) {
		viper.Reset()
		defer viper.Reset()
		viper.Set("server.http.access_log", map[string]any{
			"allow_req_body": true,
			"allow_res_body": true,
			"max_req_len":    100,
			"max_res_len":    200,
			"max_path_len":   50,
		})
		b := NewLoggerMiddlewareBuilder(func(context.Context, RequestLog) {})
		b.LoadConfig()
		if !b.allowReqBody.Load() || !b.allowResBody.Load() {
			t.Fatal("应读到 allow_*=true")
		}
		if b.maxReqLen.Load() != 100 || b.maxResLen.Load() != 200 || b.maxPathLen.Load() != 50 {
			t.Fatalf("max 值不对: %d/%d/%d", b.maxReqLen.Load(), b.maxResLen.Load(), b.maxPathLen.Load())
		}
	})

	t.Run("解码失败_保持当前值不覆盖", func(t *testing.T) {
		viper.Reset()
		defer viper.Reset()
		// max_req_len 给非数字字符串 → mapstructure 解码到 int 失败，走错误分支
		viper.Set("server.http.access_log", map[string]any{"max_req_len": "not-an-int"})
		b := NewLoggerMiddlewareBuilder(func(context.Context, RequestLog) {})
		b.AllowReqBody(true)
		b.maxReqLen.Store(999)
		b.LoadConfig()
		if !b.allowReqBody.Load() || b.maxReqLen.Load() != 999 {
			t.Fatalf("解码失败应保持当前值, 实得 allowReq=%v maxReq=%d", b.allowReqBody.Load(), b.maxReqLen.Load())
		}
	})
}

// runReq 用给定配置驱动一次真实请求过中间件，返回捕获的 RequestLog + recorder。
func runReq(t *testing.T, configure func(*LoggerMiddlewareBuilder),
	method, target string, body io.Reader, handler gin.HandlerFunc) (RequestLog, *httptest.ResponseRecorder) {
	t.Helper()
	var got RequestLog
	var called bool
	b := NewLoggerMiddlewareBuilder(func(_ context.Context, l RequestLog) { got = l; called = true })
	if configure != nil {
		configure(b)
	}
	r := gin.New()
	r.Use(b.Build())
	r.Handle(method, "/*any", handler)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, body))
	if !called {
		t.Fatal("loggerFunc 未被调用")
	}
	return got, w
}

func TestBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("默认全关_只记基础字段+status回填+duration", func(t *testing.T) {
		got, w := runReq(t, nil, http.MethodGet, "/api/x?a=1", nil, func(c *gin.Context) {
			time.Sleep(time.Millisecond) // 保证 Duration>0 稳定
			c.String(http.StatusOK, "hi")
		})
		if got.Path != "/api/x" || got.Method != http.MethodGet || got.Query != "a=1" {
			t.Fatalf("基础字段不对: %+v", got)
		}
		if got.ClientIP == "" {
			t.Fatal("ClientIP 应有值")
		}
		if got.Status != http.StatusOK || w.Code != http.StatusOK {
			t.Fatalf("status: rl=%d w=%d", got.Status, w.Code)
		}
		if got.ReqBody != "" || got.ResBody != "" {
			t.Fatalf("默认不应记 body: req=%q res=%q", got.ReqBody, got.ResBody)
		}
		if got.Duration <= 0 {
			t.Fatalf("Duration 应 >0, 实得 %v", got.Duration)
		}
	})

	t.Run("allowReqBody_记录并还原请求体", func(t *testing.T) {
		var handlerSaw string
		got, _ := runReq(t, func(b *LoggerMiddlewareBuilder) { b.AllowReqBody(true) },
			http.MethodPost, "/x", strings.NewReader("hello-body"), func(c *gin.Context) {
				data, _ := io.ReadAll(c.Request.Body) // 验证 body 已还原，handler 仍能读到
				handlerSaw = string(data)
				c.Status(http.StatusOK)
			})
		if got.ReqBody != "hello-body" {
			t.Fatalf("ReqBody=%q", got.ReqBody)
		}
		if handlerSaw != "hello-body" {
			t.Fatalf("请求体未还原, handler 读到 %q", handlerSaw)
		}
	})

	t.Run("allowReqBody_截断", func(t *testing.T) {
		got, _ := runReq(t, func(b *LoggerMiddlewareBuilder) { b.AllowReqBody(true); b.maxReqLen.Store(3) },
			http.MethodPost, "/x", strings.NewReader("hello"), func(c *gin.Context) { c.Status(http.StatusOK) })
		if got.ReqBody != "hel" {
			t.Fatalf("ReqBody 截断=%q want hel", got.ReqBody)
		}
	})

	t.Run("allowResBody_记录响应体和status", func(t *testing.T) {
		got, w := runReq(t, func(b *LoggerMiddlewareBuilder) { b.AllowResBody(true) },
			http.MethodGet, "/x", nil, func(c *gin.Context) { c.String(http.StatusCreated, "response-data") })
		if got.ResBody != "response-data" {
			t.Fatalf("ResBody=%q", got.ResBody)
		}
		if got.Status != http.StatusCreated || w.Code != http.StatusCreated {
			t.Fatalf("status: rl=%d w=%d want 201", got.Status, w.Code)
		}
	})

	t.Run("allowResBody_截断", func(t *testing.T) {
		got, _ := runReq(t, func(b *LoggerMiddlewareBuilder) { b.AllowResBody(true); b.maxResLen.Store(4) },
			http.MethodGet, "/x", nil, func(c *gin.Context) { c.String(http.StatusOK, "abcdefg") })
		if got.ResBody != "abcd" {
			t.Fatalf("ResBody 截断=%q want abcd", got.ResBody)
		}
	})

	t.Run("allowResBody_WriteString路径", func(t *testing.T) {
		got, _ := runReq(t, func(b *LoggerMiddlewareBuilder) { b.AllowResBody(true) },
			http.MethodGet, "/x", nil, func(c *gin.Context) {
				c.Status(http.StatusOK)
				_, _ = c.Writer.WriteString("via-writestring")
			})
		if got.ResBody != "via-writestring" {
			t.Fatalf("WriteString 未记录 ResBody=%q", got.ResBody)
		}
	})

	t.Run("maxPathLen_截断路径", func(t *testing.T) {
		got, _ := runReq(t, func(b *LoggerMiddlewareBuilder) { b.maxPathLen.Store(4) },
			http.MethodGet, "/abcdefgh", nil, func(c *gin.Context) { c.Status(http.StatusOK) })
		if got.Path != "/abc" {
			t.Fatalf("Path 截断=%q want /abc", got.Path)
		}
	})

	t.Run("status为0走ctx.Writer.Status回填", func(t *testing.T) {
		// allowResBody=false → responseWriter 不介入 → rl.Status 保持 0 → defer 回填
		got, _ := runReq(t, nil, http.MethodGet, "/x", nil, func(c *gin.Context) {
			c.Status(http.StatusAccepted)
		})
		if got.Status != http.StatusAccepted {
			t.Fatalf("Status 回填=%d want 202", got.Status)
		}
	})
}
