package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/webook/migrator/consts"
	"github.com/webook/migrator/domain"
	"github.com/webook/pkg/logger"
)

// ── 手写 stub AuditLogRepository ───────────────────────────
type stubAuditLogRepository struct {
	Inserted chan domain.AuditLog
}

func (m *stubAuditLogRepository) Create(_ context.Context, lg domain.AuditLog) (int64, error) {
	if m.Inserted != nil {
		m.Inserted <- lg
	}
	return 1, nil
}

func setupAuditRouter(t *testing.T, mc *stubAuditLogRepository, withActor bool, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mw := NewAuditMiddleware(mc, logger.NewNopLogger())
	r := gin.New()
	if withActor {
		r.Use(func(c *gin.Context) {
			c.Set(AuditUserIDCtxKey, int64(1001))
			c.Next()
		})
	}
	r.Use(mw.Build())
	r.GET("/migrator/tasks/:id", handler)
	r.POST("/migrator/tasks", handler)
	r.POST("/migrator/tasks/:id/start", handler)
	return r
}

// waitAudit 等异步 goroutine 把 audit 推过来；timeout 视为未调用。
func waitAudit(t *testing.T, ch chan domain.AuditLog) (domain.AuditLog, bool) {
	t.Helper()
	select {
	case lg := <-ch:
		return lg, true
	case <-time.After(500 * time.Millisecond):
		return domain.AuditLog{}, false
	}
}

func okHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": gin.H{"taskId": 42}})
}

func errHandler(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid"})
}

func TestAuditMiddleware(t *testing.T) {
	t.Run("GET 请求跳过 audit", func(t *testing.T) {
		ch := make(chan domain.AuditLog, 1)
		r := setupAuditRouter(t, &stubAuditLogRepository{Inserted: ch}, true, okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/migrator/tasks/7", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		_, called := waitAudit(t, ch)
		assert.False(t, called, "audit should NOT be inserted for GET")
	})

	t.Run("POST create 成功 → audit success + taskId from response", func(t *testing.T) {
		ch := make(chan domain.AuditLog, 1)
		r := setupAuditRouter(t, &stubAuditLogRepository{Inserted: ch}, true, okHandler)

		w := httptest.NewRecorder()
		body := []byte(`{"name":"t1","sourceDsnRef":"vault:src","sinkDsnRef":"vault:dst"}`)
		req := httptest.NewRequest(http.MethodPost, "/migrator/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		lg, called := waitAudit(t, ch)
		assert.True(t, called, "audit should be inserted for POST create")
		assert.Equal(t, "1001", lg.Actor)
		assert.Equal(t, consts.AuditActionCreate, lg.Action)
		assert.Equal(t, consts.AuditResultSuccess, lg.Result)
		assert.EqualValues(t, 42, lg.TaskId, "taskId should be extracted from response.data.taskId")
		// payload mask 验证
		assert.True(t, strings.Contains(lg.Payload, `"sourceDsnRef":"***"`), "sourceDsnRef should be masked: %s", lg.Payload)
		assert.True(t, strings.Contains(lg.Payload, `"sinkDsnRef":"***"`), "sinkDsnRef should be masked: %s", lg.Payload)
		assert.False(t, strings.Contains(lg.Payload, "vault:src"), "DSN value should not leak: %s", lg.Payload)
	})

	t.Run("POST 失败 → audit fail", func(t *testing.T) {
		ch := make(chan domain.AuditLog, 1)
		r := setupAuditRouter(t, &stubAuditLogRepository{Inserted: ch}, true, errHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/migrator/tasks", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		lg, called := waitAudit(t, ch)
		assert.True(t, called)
		assert.Equal(t, consts.AuditResultFail, lg.Result)
	})

	t.Run("缺 actor → anonymous", func(t *testing.T) {
		ch := make(chan domain.AuditLog, 1)
		r := setupAuditRouter(t, &stubAuditLogRepository{Inserted: ch}, false, okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/migrator/tasks", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		lg, called := waitAudit(t, ch)
		assert.True(t, called)
		assert.Equal(t, AuditActorAnonymous, lg.Actor)
	})

	t.Run("path :id 优先于 response data.taskId", func(t *testing.T) {
		ch := make(chan domain.AuditLog, 1)
		r := setupAuditRouter(t, &stubAuditLogRepository{Inserted: ch}, true, okHandler)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/migrator/tasks/123/start", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		lg, called := waitAudit(t, ch)
		assert.True(t, called)
		assert.EqualValues(t, 123, lg.TaskId, "path :id should win over response taskId")
		// /start 无 phase body → 默认 start_full
		assert.Equal(t, consts.AuditActionStartFull, lg.Action)
	})

	t.Run("非 dsnRef 字段不 mask", func(t *testing.T) {
		ch := make(chan domain.AuditLog, 1)
		r := setupAuditRouter(t, &stubAuditLogRepository{Inserted: ch}, true, okHandler)

		w := httptest.NewRecorder()
		body := []byte(`{"name":"public-name","mode":"cdc","sinkType":"es"}`)
		req := httptest.NewRequest(http.MethodPost, "/migrator/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		lg, called := waitAudit(t, ch)
		assert.True(t, called)
		assert.True(t, strings.Contains(lg.Payload, "public-name"), "non-sensitive fields should pass through: %s", lg.Payload)
		assert.False(t, strings.Contains(lg.Payload, "***"), "no field should be masked when no dsnRef: %s", lg.Payload)
	})
}

func TestActionFor(t *testing.T) {
	testCases := []struct {
		name, path, method, body, want string
	}{
		{"GET 跳过", "/migrator/tasks/1", http.MethodGet, "", AuditActionUnknown},
		{"create", "/migrator/tasks", http.MethodPost, "{}", consts.AuditActionCreate},
		{"preflight", "/migrator/preflight", http.MethodPost, "{}", consts.AuditActionPreflight},
		{"start full(指定)", "/migrator/tasks/1/start", http.MethodPost, `{"phase":"full"}`, consts.AuditActionStartFull},
		{"start full(空 body 兜底)", "/migrator/tasks/1/start", http.MethodPost, "{}", consts.AuditActionStartFull},
		{"start incr", "/migrator/tasks/1/start", http.MethodPost, `{"phase":"incr"}`, consts.AuditActionStartIncr},
		{"pause", "/migrator/tasks/1/pause", http.MethodPost, "", consts.AuditActionPause},
		{"throttle", "/migrator/tasks/1/throttle", http.MethodPost, "{}", consts.AuditActionThrottle},
		{"gray", "/migrator/tasks/1/gray", http.MethodPost, `{"percent":10}`, consts.AuditActionSetGray},
		{"switch 推进(无 action)", "/migrator/tasks/1/switch", http.MethodPost, `{"stage":"SRC_FIRST"}`, consts.AuditActionSetStageSRCFirst},
		{"switch propose", "/migrator/tasks/1/switch", http.MethodPost, `{"stage":"DST_ONLY","action":"propose"}`, consts.AuditActionCutoverPropose},
		{"switch approve", "/migrator/tasks/1/switch", http.MethodPost, `{"stage":"DST_ONLY","action":"approve"}`, consts.AuditActionCutoverApprove},
		{"switch rollback", "/migrator/tasks/1/switch", http.MethodPost, `{"action":"rollback"}`, consts.AuditActionRollback},
		{"verify", "/migrator/tasks/1/verify", http.MethodPost, "{}", consts.AuditActionVerify},
		{"repair", "/migrator/tasks/1/repair", http.MethodPost, "{}", consts.AuditActionRepair},
		{"replay-dl", "/migrator/tasks/1/replay-dl", http.MethodPost, "{}", consts.AuditActionReplayDL},
		{"未知路径 → unknown", "/migrator/tasks/1/foobar", http.MethodPost, "{}", AuditActionUnknown},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, actionFor(c.path, c.method, []byte(c.body)))
		})
	}
}
