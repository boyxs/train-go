package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/boyxs/train-go/webook/migrator/domain"
	migratorerrs "github.com/boyxs/train-go/webook/migrator/errs"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/service"
)

// ── 手写 stub Service ──────────────────────────────────────
type stubTaskService struct {
	CreateFn        func(ctx context.Context, req service.CreateReq) (int64, error)
	GetFn           func(ctx context.Context, id int64) (domain.Task, error)
	ListFn          func(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error)
	SetThrottleFn   func(ctx context.Context, id int64, cfg domain.ThrottleConfig) error
	ClearThrottleFn func(ctx context.Context, id int64) error
}

func (s *stubTaskService) Create(ctx context.Context, r service.CreateReq) (int64, error) {
	if s.CreateFn != nil {
		return s.CreateFn(ctx, r)
	}
	return 0, nil
}
func (s *stubTaskService) Get(ctx context.Context, id int64) (domain.Task, error) {
	if s.GetFn != nil {
		return s.GetFn(ctx, id)
	}
	return domain.Task{Id: id, TablesJSON: `[{"src":"article","dst":"article_v1","partitionKey":"id"}]`}, nil
}
func (s *stubTaskService) List(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error) {
	if s.ListFn != nil {
		return s.ListFn(ctx, opts)
	}
	return nil, 0, nil
}
func (s *stubTaskService) UpdateStatus(_ context.Context, _ int64, _ domain.TaskStatus) error {
	return nil
}

// throttle 三方法默认模拟"存储未装配"（Set/Clear → 501 sentinel），与真实 TaskService
// 未注入 ThrottleRepository 的行为一致；需要成功路径的用例注入 Fn 覆盖。
func (s *stubTaskService) SetThrottle(ctx context.Context, id int64, cfg domain.ThrottleConfig) error {
	if s.SetThrottleFn != nil {
		return s.SetThrottleFn(ctx, id, cfg)
	}
	return migratorerrs.ErrThrottleNotConfigured
}
func (s *stubTaskService) ClearThrottle(ctx context.Context, id int64) error {
	if s.ClearThrottleFn != nil {
		return s.ClearThrottleFn(ctx, id)
	}
	return migratorerrs.ErrThrottleNotConfigured
}
func (s *stubTaskService) GetThrottle(_ context.Context, _ int64) (domain.ThrottleConfig, bool, error) {
	return domain.ThrottleConfig{}, false, nil
}

func setupRouter(svc service.TaskService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 引擎类参数传 nil — 测试只覆盖 CRUD，control endpoint 未调用
	NewTaskHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil).RegisterRoutes(r)
	return r
}

func validBody() []byte {
	b, _ := json.Marshal(map[string]any{
		"name":         "article_to_es_v1",
		"mode":         "cdc",
		"kind":         "heterogeneous",
		"sourceDsnRef": "vault:src",
		"sinkType":     "es",
		"sinkDsnRef":   "vault:dst",
		"tables":       []map[string]any{{"src": "article", "dst": "article_v1", "partitionKey": "id"}},
	})
	return b
}

func decodeResult(t *testing.T, w *httptest.ResponseRecorder) Result {
	t.Helper()
	var resp Result
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestTaskHandler_Create(t *testing.T) {
	t.Run("成功 200 + code=0", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			CreateFn: func(ctx context.Context, req service.CreateReq) (int64, error) {
				assert.Equal(t, "article_to_es_v1", req.Name)
				assert.Equal(t, domain.ModeCDC, req.Mode)
				return 42, nil
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/migrator/tasks", bytes.NewReader(validBody()))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		resp := decodeResult(t, w)
		// ginx.Wrap 成功路径 code 由业务填，taskService 不设 Code → 默认零值 0 == 成功
		data := resp.Data.(map[string]any)
		assert.EqualValues(t, 42, data["taskId"])
	})

	t.Run("schema 校验失败 400", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			CreateFn: func(ctx context.Context, _ service.CreateReq) (int64, error) {
				t.Fatal("service.Create should not be called")
				return 0, nil
			},
		})
		w := httptest.NewRecorder()
		// 缺 name
		body, _ := json.Marshal(map[string]any{"mode": "cdc", "kind": "heterogeneous"})
		req := httptest.NewRequest("POST", "/migrator/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		// ginx.WrapReq 反序列化/验证失败 → 400 "参数错误"
		assert.Equal(t, 400, w.Code)
		resp := decodeResult(t, w)
		assert.Equal(t, 400, resp.Code)
		assert.Equal(t, "参数错误", resp.Msg)
	})

	t.Run("业务校验失败 400 (ErrInvalidArgument)", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			CreateFn: func(ctx context.Context, _ service.CreateReq) (int64, error) {
				return 0, migratorerrs.ErrInvalidArgument
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/migrator/tasks", bytes.NewReader(validBody()))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		// *errs.Error.Code = 400 → HTTP 400 + Msg "参数不合法"
		assert.Equal(t, 400, w.Code)
		resp := decodeResult(t, w)
		assert.Equal(t, 400, resp.Code)
		assert.Equal(t, "参数不合法", resp.Msg)
	})

	t.Run("name 重复 409 (ErrDuplicateTaskName)", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			CreateFn: func(ctx context.Context, _ service.CreateReq) (int64, error) {
				return 0, migratorerrs.ErrDuplicateTaskName
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/migrator/tasks", bytes.NewReader(validBody()))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, 409, w.Code)
		resp := decodeResult(t, w)
		assert.Equal(t, 409, resp.Code)
		assert.Equal(t, "迁移任务名已存在", resp.Msg)
	})

	t.Run("系统错误 500", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			CreateFn: func(ctx context.Context, _ service.CreateReq) (int64, error) {
				return 0, assert.AnError
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/migrator/tasks", bytes.NewReader(validBody()))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, 500, w.Code)
		resp := decodeResult(t, w)
		assert.Equal(t, 500, resp.Code)
		assert.Equal(t, "系统错误", resp.Msg)
	})
}

func TestTaskHandler_Get(t *testing.T) {
	t.Run("命中 200", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			GetFn: func(ctx context.Context, id int64) (domain.Task, error) {
				return domain.Task{Id: id, Name: "t1"}, nil
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/migrator/tasks/7", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("未找到 404", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			GetFn: func(ctx context.Context, id int64) (domain.Task, error) {
				return domain.Task{}, migratorerrs.ErrTaskNotFound
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/migrator/tasks/999", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, 404, w.Code)
		resp := decodeResult(t, w)
		assert.Equal(t, 404, resp.Code)
		assert.Equal(t, "迁移任务不存在", resp.Msg)
	})

	t.Run("id 不合法 400", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			GetFn: func(ctx context.Context, _ int64) (domain.Task, error) {
				t.Fatal("Get should not be called")
				return domain.Task{}, nil
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/migrator/tasks/abc", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, 400, w.Code)
	})
}

func TestTaskHandler_List(t *testing.T) {
	t.Run("分页正常", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			ListFn: func(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error) {
				assert.Equal(t, 0, opts.Offset)
				assert.Equal(t, 10, opts.Limit)
				return []domain.Task{{Id: 1}, {Id: 2}}, 2, nil
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/migrator/tasks?offset=0&limit=10", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		resp := decodeResult(t, w)
		data := resp.Data.(map[string]any)
		assert.EqualValues(t, 2, data["total"])
	})

	t.Run("status 过滤", func(t *testing.T) {
		r := setupRouter(&stubTaskService{
			ListFn: func(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error) {
				assert.NotNil(t, opts.Status)
				assert.Equal(t, domain.TaskStatusIncrRunning, *opts.Status)
				return []domain.Task{}, 0, nil
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/migrator/tasks?status=3", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("limit 超界回落默认", func(t *testing.T) {
		var capturedLimit int
		r := setupRouter(&stubTaskService{
			ListFn: func(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error) {
				capturedLimit = opts.Limit
				return []domain.Task{}, 0, nil
			},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/migrator/tasks?limit=99999", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, 50, capturedLimit)
	})
}
