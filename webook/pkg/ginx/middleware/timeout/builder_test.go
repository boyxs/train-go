package timeout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMiddleware_Deadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var hasDeadline bool
	r := gin.New()
	r.Use(NewMiddlewareBuilder(5 * time.Second).ExemptPrefix("/chat").Build())
	probe := func(c *gin.Context) {
		_, hasDeadline = c.Request.Context().Deadline()
		c.Status(http.StatusOK)
	}
	r.GET("/x", probe)
	r.GET("/chat/sse", probe)

	// 非豁免路径 → 设了 deadline
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if !hasDeadline {
		t.Error("非豁免路径应设 deadline")
	}

	// 豁免前缀 /chat → 不设 deadline（SSE 不能被切断）
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/chat/sse", nil))
	if hasDeadline {
		t.Error("豁免路径不应设 deadline")
	}
}

func TestNewMiddlewareBuilder_defaultTimeout(t *testing.T) {
	if b := NewMiddlewareBuilder(0); b.timeout != defaultTimeout {
		t.Errorf("d<=0 应兜底 %v，实得 %v", defaultTimeout, b.timeout)
	}
	if b := NewMiddlewareBuilder(3 * time.Second); b.timeout != 3*time.Second {
		t.Errorf("d>0 应用传入值，实得 %v", b.timeout)
	}
}

// TestMiddleware_RealRequest 用真实 HTTP server（真实 TCP）+ 真实 client 打请求，
// 验证当前「软超时」中间件的实际行为——软超时只挂 ctx deadline，是否切断取决于 handler 是否守 ctx。
func TestMiddleware_RealRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const to = 200 * time.Millisecond   // 中间件超时（server.http.timeout）
	const slow = 800 * time.Millisecond // handler 慢操作耗时

	newServer := func(handler gin.HandlerFunc) *httptest.Server {
		r := gin.New()
		r.Use(NewMiddlewareBuilder(to).ExemptPrefix("/chat").Build())
		r.GET("/t", handler)
		r.GET("/chat/t", handler)
		return httptest.NewServer(r)
	}
	do := func(t *testing.T, srv *httptest.Server, path string) (int, time.Duration) {
		t.Helper()
		c := &http.Client{Timeout: 5 * time.Second} // client 超时放大，确保不是它在切
		start := time.Now()
		resp, err := c.Get(srv.URL + path)
		el := time.Since(start)
		if err != nil {
			t.Fatalf("请求 %s 出错: %v", path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode, el
	}

	// 守 ctx 的 handler：自己 select ctx.Done() → 软超时到点被切，快速返回 504
	t.Run("守ctx的handler_到点被切", func(t *testing.T) {
		srv := newServer(func(c *gin.Context) {
			select {
			case <-time.After(slow):
				c.JSON(http.StatusOK, gin.H{"ok": true})
			case <-c.Request.Context().Done():
				c.JSON(http.StatusGatewayTimeout, gin.H{"code": 504, "msg": "handler 感知超时"})
			}
		})
		defer srv.Close()
		code, el := do(t, srv, "/t")
		t.Logf("守ctx handler → HTTP %d，耗时 %v", code, el.Round(10*time.Millisecond))
		if code != http.StatusGatewayTimeout {
			t.Fatalf("守ctx handler 应到点返回 504，实得 %d", code)
		}
		if el > slow/2 {
			t.Fatalf("应在 ~%v 被切，实际 %v", to, el)
		}
	})

	// 忽略 ctx 的 handler：软超时切不掉，拖满慢操作才返回（这正是「设置时间没生效」的场景）
	t.Run("忽略ctx的handler_软超时切不掉", func(t *testing.T) {
		srv := newServer(func(c *gin.Context) {
			time.Sleep(slow) // 完全不看 ctx
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		defer srv.Close()
		code, el := do(t, srv, "/t")
		t.Logf("忽略ctx handler → HTTP %d，耗时 %v（软超时未切断）", code, el.Round(10*time.Millisecond))
		if code != http.StatusOK {
			t.Fatalf("软超时不改写响应，应返回 handler 的 200，实得 %d", code)
		}
		if el < slow/2 {
			t.Fatalf("软超时对忽略 ctx 的 handler 不应切断，应拖到 ~%v，实际 %v", slow, el)
		}
	})

	// 普通 handler 的真实写法：不手写 select，只把 ctx 传给 ctx-aware 的下游调用（gorm/redis/grpc）→ 同样被切
	t.Run("普通handler_ctx传下游_会被切", func(t *testing.T) {
		// ctxAwareOp 模拟 db.WithContext(ctx).Find(...) 这类库调用：内部尊重 ctx，到点返回 DeadlineExceeded
		ctxAwareOp := func(ctx context.Context) error {
			select {
			case <-time.After(slow):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		srv := newServer(func(c *gin.Context) {
			// 正常写法：把请求 ctx 传下去，拿到错误就返回；没有任何手写的 select ctx.Done()
			if err := ctxAwareOp(c.Request.Context()); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		defer srv.Close()
		code, el := do(t, srv, "/t")
		t.Logf("普通handler(ctx传下游) → HTTP %d，耗时 %v（软超时到点，下游返 DeadlineExceeded）", code, el.Round(10*time.Millisecond))
		if el > slow/2 {
			t.Fatalf("ctx 传到位的普通 handler 应在 ~%v 被切，实际 %v（说明下游没吃 ctx）", to, el)
		}
		// 关键：软超时不代写 504，客户端拿到的是下游 ctx 错误映射出的码（这里 500），不是干净的 504
		if code != http.StatusInternalServerError {
			t.Fatalf("软超时返回的是下游错误(此处映射为 500)，非 504，实得 %d", code)
		}
	})

	// 豁免前缀：完全不设 deadline、直连
	t.Run("豁免路径_不设deadline", func(t *testing.T) {
		var hasDeadline bool
		srv := newServer(func(c *gin.Context) {
			_, hasDeadline = c.Request.Context().Deadline()
			c.Status(http.StatusOK)
		})
		defer srv.Close()
		if code, _ := do(t, srv, "/chat/t"); code != http.StatusOK {
			t.Fatalf("豁免路径应正常放行 200，实得 %d", code)
		}
		if hasDeadline {
			t.Fatal("豁免路径不应设 deadline")
		}
	})
}
