package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/webook/migrator/domain"
	migratorerrs "github.com/webook/migrator/errs"
	"github.com/webook/migrator/pipeline/dsn"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/repository/cache"
	"github.com/webook/migrator/service"
	"github.com/webook/migrator/service/full"
	"github.com/webook/migrator/service/incr"
	"github.com/webook/migrator/service/switching"
	"github.com/webook/migrator/service/verify"
	"github.com/webook/pkg/logger"
)

type stubFullEngine struct {
	full.FullEngine
	RunFn       func(ctx context.Context, taskId int64, shards []source.ShardSpec) error
	PauseFn     func(taskId int64) error
	IsRunningFn func(taskId int64) bool
}

func (s *stubFullEngine) Run(ctx context.Context, taskId int64, shards []source.ShardSpec) error {
	if s.RunFn != nil {
		return s.RunFn(ctx, taskId, shards)
	}
	return nil
}
func (s *stubFullEngine) Pause(id int64) error {
	if s.PauseFn != nil {
		return s.PauseFn(id)
	}
	return nil
}
func (s *stubFullEngine) IsRunning(id int64) bool {
	if s.IsRunningFn != nil {
		return s.IsRunningFn(id)
	}
	return false
}

type stubIncrEngine struct {
	incr.IncrEngine
	RunFn       func(ctx context.Context, taskId int64) error
	PauseFn     func(taskId int64) error
	LagFn       func(taskId int64) (int64, error)
	LagDstFn    func(taskId int64) (int64, error)
	IsRunningFn func(taskId int64) bool
}

func (s *stubIncrEngine) Run(ctx context.Context, taskId int64) error {
	if s.RunFn != nil {
		return s.RunFn(ctx, taskId)
	}
	return nil
}
func (s *stubIncrEngine) Pause(id int64) error {
	if s.PauseFn != nil {
		return s.PauseFn(id)
	}
	return nil
}
func (s *stubIncrEngine) IsRunning(id int64) bool {
	if s.IsRunningFn != nil {
		return s.IsRunningFn(id)
	}
	return false
}
func (s *stubIncrEngine) Lag(id int64) (int64, error) {
	if s.LagFn != nil {
		return s.LagFn(id)
	}
	return 0, nil
}
func (s *stubIncrEngine) LagDst(id int64) (int64, error) {
	if s.LagDstFn != nil {
		return s.LagDstFn(id)
	}
	return -1, nil
}

type stubVerifyEngine struct {
	verify.VerifyEngine
	SampleFn       func(ctx context.Context, id int64, rate float64) (int64, error)
	FullFn         func(ctx context.Context, id int64) (int64, error)
	RepairFn       func(ctx context.Context, id int64, strategy verify.RepairStrategy, ids []int64) (int64, error)
	ListMismatchFn func(ctx context.Context, id int64, offset, limit int) ([]domain.ValidateLog, int64, error)
}

func (s *stubVerifyEngine) Sample(ctx context.Context, id int64, rate float64) (int64, error) {
	return s.SampleFn(ctx, id, rate)
}
func (s *stubVerifyEngine) Full(ctx context.Context, id int64) (int64, error) {
	return s.FullFn(ctx, id)
}
func (s *stubVerifyEngine) Repair(ctx context.Context, id int64, strategy verify.RepairStrategy, ids []int64) (int64, error) {
	return s.RepairFn(ctx, id, strategy, ids)
}
func (s *stubVerifyEngine) ListMismatch(ctx context.Context, id int64, offset, limit int) ([]domain.ValidateLog, int64, error) {
	return s.ListMismatchFn(ctx, id, offset, limit)
}

type stubSwitchService struct {
	switching.SwitchService
	SetGrayFn  func(ctx context.Context, id int64, p int) error
	SetStageFn func(ctx context.Context, id int64, stage domain.Stage, propose, approve string) error
	RollbackFn func(ctx context.Context, id int64) error
}

func (s *stubSwitchService) SetGray(ctx context.Context, id int64, p int) error {
	return s.SetGrayFn(ctx, id, p)
}
func (s *stubSwitchService) SetStage(ctx context.Context, id int64, stage domain.Stage, propose, approve string) error {
	return s.SetStageFn(ctx, id, stage, propose, approve)
}
func (s *stubSwitchService) Rollback(ctx context.Context, id int64) error {
	return s.RollbackFn(ctx, id)
}

// stubReplayService — 死信重放 service stub。
type stubReplayService struct {
	ReplayFn func(ctx context.Context, taskId int64, limit int) (int64, int64, error)
}

func (s *stubReplayService) ReplayDeadLetters(ctx context.Context, taskId int64, limit int) (int64, int64, error) {
	if s.ReplayFn != nil {
		return s.ReplayFn(ctx, taskId, limit)
	}
	return 0, 0, nil
}

// stubSourceFactory：handler 测试用的 factory stub。
// handler 只用 BuildFullSrc 路径（PKRange 切片）。
type stubSourceFactory struct {
	BuildSrcFn func(ctx context.Context, task domain.Task) (source.FullSource, error)
	BuildDstFn func(ctx context.Context, task domain.Task) (source.FullSource, error)
}

func (f *stubSourceFactory) BuildFullSrc(ctx context.Context, task domain.Task, _ int) (source.FullSource, error) {
	if f.BuildSrcFn != nil {
		return f.BuildSrcFn(ctx, task)
	}
	return nil, nil
}
func (f *stubSourceFactory) BuildIncrSrc(_ context.Context, _ domain.Task, _ int) (source.IncrSource, error) {
	return nil, nil
}
func (f *stubSourceFactory) BuildDst(ctx context.Context, task domain.Task, _ int) (source.FullSource, error) {
	if f.BuildDstFn != nil {
		return f.BuildDstFn(ctx, task)
	}
	return nil, nil
}

// ── 测试 setup helper ──────────────────────────────────────
type stubs struct {
	fe  *stubFullEngine
	ie  *stubIncrEngine
	ve  *stubVerifyEngine
	ss  *stubSwitchService
	rp  *stubReplayService
	svc *stubTaskService
}

func setupControlRouter(t *testing.T) (*gin.Engine, *stubs) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &stubs{
		fe:  &stubFullEngine{},
		ie:  &stubIncrEngine{},
		ve:  &stubVerifyEngine{},
		ss:  &stubSwitchService{},
		rp:  &stubReplayService{},
		svc: &stubTaskService{},
	}
	srcFac := &stubSourceFactory{}
	h := NewTaskHandler(s.svc, s.fe, s.ie, s.ve, s.ss, s.rp, srcFac, nil, logger.NewNopLogger())
	r := gin.New()
	h.RegisterRoutes(r)
	return r, s
}

func doPOST(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	} else {
		raw = []byte("{}")
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func doGET(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w
}

// ── 11 个 endpoint 测试 ────────────────────────────────────

func TestControl_Preflight(t *testing.T) {
	t.Run("源库 binlog_format=ROW + gtid_mode=ON + 表有 PK → ready=true", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer mockDB.Close()
		gormDB, err := gorm.Open(mysql.New(mysql.Config{
			Conn: mockDB, SkipInitializeWithVersion: true,
		}), &gorm.Config{})
		require.NoError(t, err)

		mock.ExpectQuery("@@global.binlog_format").
			WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow("ROW"))
		mock.ExpectQuery("@@global.gtid_mode").
			WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow("ON"))
		mock.ExpectQuery("information_schema.STATISTICS").
			WithArgs("article").
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(int64(1)))

		r := newPreflightRouter(t, dsn.NewStaticResolver(gormDB))
		w := doPOST(t, r, "/migrator/preflight", map[string]any{
			"sourceDsnRef": "vault:src",
			"tables":       []string{"article"},
		})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"ready":true`)
		assert.Contains(t, w.Body.String(), `"binlog_format":"ROW"`)
		assert.Contains(t, w.Body.String(), `"gtid_mode":"ON"`)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("表无 PK → ready=false 且 tables_missing_pk 含该表", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer mockDB.Close()
		gormDB, err := gorm.Open(mysql.New(mysql.Config{
			Conn: mockDB, SkipInitializeWithVersion: true,
		}), &gorm.Config{})
		require.NoError(t, err)

		mock.ExpectQuery("@@global.binlog_format").
			WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow("ROW"))
		mock.ExpectQuery("@@global.gtid_mode").
			WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow("ON"))
		mock.ExpectQuery("information_schema.STATISTICS").WithArgs("no_pk_table").
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(int64(0)))

		r := newPreflightRouter(t, dsn.NewStaticResolver(gormDB))
		w := doPOST(t, r, "/migrator/preflight", map[string]any{
			"sourceDsnRef": "vault:src",
			"tables":       []string{"no_pk_table"},
		})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"ready":false`)
		assert.Contains(t, w.Body.String(), `"tables_missing_pk":["no_pk_table"]`)
	})

	t.Run("resolver 未注入 → 501", func(t *testing.T) {
		r := newPreflightRouter(t, nil)
		w := doPOST(t, r, "/migrator/preflight", map[string]any{
			"sourceDsnRef": "vault:src", "tables": []string{"article"},
		})
		assert.Equal(t, http.StatusNotImplemented, w.Code)
	})
}

// newPreflightRouter 单独构造 router，handler 只关心 resolver；其它依赖留 nil。
func newPreflightRouter(t *testing.T, r dsn.Resolver) *gin.Engine {
	t.Helper()
	h := NewTaskHandler(nil, nil, nil, nil, nil, nil, nil, r, logger.NewNopLogger())
	gin.SetMode(gin.TestMode)
	g := gin.New()
	h.RegisterRoutes(g)
	return g
}

func TestControl_Start(t *testing.T) {
	t.Run("full → 异步启动 FullEngine.Run", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var ran int32
		s.fe.RunFn = func(_ context.Context, taskId int64, shards []source.ShardSpec) error {
			atomic.StoreInt32(&ran, 1)
			assert.Equal(t, int64(42), taskId)
			assert.Len(t, shards, 1) // 默认单分片
			return nil
		}
		w := doPOST(t, r, "/migrator/tasks/42/start", map[string]any{"phase": "full"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Eventually(t, func() bool { return atomic.LoadInt32(&ran) == 1 },
			500*time.Millisecond, 20*time.Millisecond)
	})

	t.Run("incr → 异步启动 IncrEngine.Run", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var ran int32
		s.ie.RunFn = func(_ context.Context, taskId int64) error {
			atomic.StoreInt32(&ran, 1)
			return nil
		}
		w := doPOST(t, r, "/migrator/tasks/42/start", map[string]any{"phase": "incr"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Eventually(t, func() bool { return atomic.LoadInt32(&ran) == 1 },
			500*time.Millisecond, 20*time.Millisecond)
	})

	t.Run("非法 phase → 400", func(t *testing.T) {
		r, _ := setupControlRouter(t)
		w := doPOST(t, r, "/migrator/tasks/42/start", map[string]any{"phase": "foobar"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestControl_Pause(t *testing.T) {
	t.Run("full 或 incr 任一停成功 → 200", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.fe.PauseFn = func(_ int64) error { return nil }
		s.ie.PauseFn = func(_ int64) error { return errors.New("not running") }
		w := doPOST(t, r, "/migrator/tasks/42/pause", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("full 和 incr 都没在跑 → 409", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.fe.PauseFn = func(_ int64) error { return errors.New("not running") }
		s.ie.PauseFn = func(_ int64) error { return errors.New("not running") }
		w := doPOST(t, r, "/migrator/tasks/42/pause", nil)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestControl_Throttle(t *testing.T) {
	// newThrottleRouter 真 TaskService + 真 ThrottleRepository（miniredis），
	// 覆盖 handler→service→repository→cache 全链（TaskRepository 留 nil — throttle 路径不触 task 表）。
	newThrottleRouter := func(t *testing.T, mr *miniredis.Miniredis) *gin.Engine {
		t.Helper()
		gin.SetMode(gin.TestMode)
		cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		svc := service.NewTaskService(nil,
			repository.NewThrottleRepository(cache.NewRedisThrottleCache(cli)),
			logger.NewNopLogger())
		h := NewTaskHandler(svc, nil, nil, nil, nil, nil, nil, nil, logger.NewNopLogger())
		r := gin.New()
		h.RegisterRoutes(r)
		return r
	}

	t.Run("throttle 存储未装配 → 501", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		svc := service.NewTaskService(nil, nil, logger.NewNopLogger())
		h := NewTaskHandler(svc, nil, nil, nil, nil, nil, nil, nil, logger.NewNopLogger())
		r := gin.New()
		h.RegisterRoutes(r)
		w := doPOST(t, r, "/migrator/tasks/42/throttle", map[string]any{"qps": 10000})
		assert.Equal(t, http.StatusNotImplemented, w.Code)
	})

	t.Run("有 ThrottleRepository → 写 cache + 返 applied_on=next_start", func(t *testing.T) {
		mr := miniredis.RunT(t)
		r := newThrottleRouter(t, mr)
		w := doPOST(t, r, "/migrator/tasks/42/throttle", map[string]any{"qps": 5000, "shard_workers": 8})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"applied_on":"next_start"`)
		assert.True(t, mr.Exists("migrator:throttle:42"))
		v, _ := mr.Get("migrator:throttle:42")
		assert.Contains(t, v, `"qps":5000`)
		assert.Contains(t, v, `"shard_workers":8`)
	})

	t.Run("qps<=0 + workers<=0 → 清空配置（恢复默认）", func(t *testing.T) {
		mr := miniredis.RunT(t)
		require.NoError(t, mr.Set("migrator:throttle:42", `{"qps":5000}`))
		r := newThrottleRouter(t, mr)
		w := doPOST(t, r, "/migrator/tasks/42/throttle", map[string]any{"qps": 0})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"cleared":true`)
		assert.False(t, mr.Exists("migrator:throttle:42"))
	})
}

func TestControl_SetGray(t *testing.T) {
	t.Run("成功", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var captured int
		s.ss.SetGrayFn = func(_ context.Context, _ int64, p int) error {
			captured = p
			return nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/gray", map[string]any{"percent": 50})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 50, captured)
	})

	t.Run("超界 → 400 + 业务消息", func(t *testing.T) {
		r, s := setupControlRouter(t)
		// stub 模拟 SwitchService.SetGray 的真实校验(handler 不再用 binding 拦)
		s.ss.SetGrayFn = func(_ context.Context, _ int64, p int) error {
			if p < 0 || p > 100 {
				return migratorerrs.ErrInvalidGrayPercent
			}
			return nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/gray", map[string]any{"percent": 200})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "灰度比例必须在 0-100 之间")
	})
}

func TestControl_SetSwitch(t *testing.T) {
	t.Run("SetStage 调 SwitchService.SetStage", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var captured domain.Stage
		s.ss.SetStageFn = func(_ context.Context, _ int64, stage domain.Stage, _, _ string) error {
			captured = stage
			return nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/switch", map[string]any{"stage": "SRC_FIRST"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, domain.StageSrcFirst, captured)
	})

	t.Run("Rollback 调 SwitchService.Rollback", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var rolled bool
		s.ss.RollbackFn = func(_ context.Context, _ int64) error {
			rolled = true
			return nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/switch", map[string]any{"stage": "ignored", "action": "rollback"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, rolled)
	})

	t.Run("非法 stage → 400", func(t *testing.T) {
		r, _ := setupControlRouter(t)
		w := doPOST(t, r, "/migrator/tasks/1/switch", map[string]any{"stage": "foobar"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestControl_Lag(t *testing.T) {
	t.Run("成功返回 srcLag + dstLag + lagMs", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.ie.LagFn = func(_ int64) (int64, error) { return 1234, nil }
		s.ie.LagDstFn = func(_ int64) (int64, error) { return 5678, nil }
		w := doGET(t, r, "/migrator/tasks/42/lag")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"lagMs":1234`)
		assert.Contains(t, w.Body.String(), `"srcLagMs":1234`)
		assert.Contains(t, w.Body.String(), `"dstLagMs":5678`)
	})

	t.Run("src 不可用但 dst 可用 → 200 srcLagMs=-1", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.ie.LagFn = func(_ int64) (int64, error) { return 0, errors.New("LagReporter not implemented") }
		s.ie.LagDstFn = func(_ int64) (int64, error) { return 999, nil }
		w := doGET(t, r, "/migrator/tasks/42/lag")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"srcLagMs":-1`)
		assert.Contains(t, w.Body.String(), `"dstLagMs":999`)
	})

	t.Run("src 和 dst 都不可用 → 503", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.ie.LagFn = func(_ int64) (int64, error) { return 0, errors.New("LagReporter not implemented") }
		s.ie.LagDstFn = func(_ int64) (int64, error) { return 0, errors.New("task not running") }
		w := doGET(t, r, "/migrator/tasks/42/lag")
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestControl_Verify(t *testing.T) {
	t.Run("full mode 调 VerifyEngine.Full", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.ve.FullFn = func(_ context.Context, _ int64) (int64, error) { return 7, nil }
		w := doPOST(t, r, "/migrator/tasks/1/verify", map[string]any{"mode": "full"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"mismatchCount":7`)
	})

	t.Run("sample mode + 默认 rate=0.01", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var captured float64
		s.ve.SampleFn = func(_ context.Context, _ int64, rate float64) (int64, error) {
			captured = rate
			return 0, nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/verify", map[string]any{"mode": "sample"})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.InDelta(t, 0.01, captured, 1e-9)
	})
}

func TestControl_Mismatch(t *testing.T) {
	r, s := setupControlRouter(t)
	s.ve.ListMismatchFn = func(_ context.Context, taskId int64, offset, limit int) ([]domain.ValidateLog, int64, error) {
		assert.Equal(t, int64(42), taskId)
		assert.Equal(t, 0, offset)
		assert.Equal(t, 50, limit)
		return []domain.ValidateLog{{Id: 1, BizId: "100"}}, 1, nil
	}
	w := doGET(t, r, "/migrator/tasks/42/mismatch")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"total":1`)
}

func TestControl_Repair(t *testing.T) {
	t.Run("mark_only 调 VerifyEngine.Repair", func(t *testing.T) {
		r, s := setupControlRouter(t)
		s.ve.RepairFn = func(_ context.Context, _ int64, strategy verify.RepairStrategy, ids []int64) (int64, error) {
			assert.Equal(t, verify.RepairMarkOnly, strategy)
			assert.Equal(t, []int64{1, 2, 3}, ids)
			return int64(len(ids)), nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/repair", map[string]any{
			"strategy": "mark_only", "ids": []int64{1, 2, 3},
		})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"repaired":3`)
	})

	t.Run("非法 strategy → 400", func(t *testing.T) {
		r, _ := setupControlRouter(t)
		w := doPOST(t, r, "/migrator/tasks/1/repair", map[string]any{
			"strategy": "foo", "ids": []int64{1},
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestControl_ReplayDL handler 只做转发；重放业务行为（payload 损坏 / Sink 失败 /
// MarkReplayed）由 service/replay 包单测覆盖。
func TestControl_ReplayDL(t *testing.T) {
	t.Run("转发 ReplayService 并透传计数", func(t *testing.T) {
		r, s := setupControlRouter(t)
		var capturedLimit int
		s.rp.ReplayFn = func(_ context.Context, taskId int64, limit int) (int64, int64, error) {
			assert.Equal(t, int64(1), taskId)
			capturedLimit = limit
			return 3, 1, nil
		}
		w := doPOST(t, r, "/migrator/tasks/1/replay-dl", map[string]any{"limit": 1000})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"replayed":3`)
		assert.Contains(t, w.Body.String(), `"failed":1`)
		assert.Equal(t, 1000, capturedLimit)
	})

	t.Run("replay service 未注入 → 501", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h := NewTaskHandler(nil, nil, nil, nil, nil, nil, nil, nil, logger.NewNopLogger())
		r := gin.New()
		h.RegisterRoutes(r)
		w := doPOST(t, r, "/migrator/tasks/1/replay-dl", map[string]any{"limit": 100})
		assert.Equal(t, http.StatusNotImplemented, w.Code)
	})
}

func TestControl_TaskIdInvalid(t *testing.T) {
	r, _ := setupControlRouter(t)
	// :id 非数字 → 400
	w := doPOST(t, r, "/migrator/tasks/abc/pause", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
