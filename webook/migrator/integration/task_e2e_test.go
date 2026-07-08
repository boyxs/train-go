package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/migrator/repository/dao"
	"github.com/boyxs/train-go/webook/migrator/service"
	"github.com/boyxs/train-go/webook/migrator/web"
	"github.com/boyxs/train-go/webook/migrator/web/middleware"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// TaskE2ESuite 端到端：拼真实 mysql + redis + handler + audit middleware，
// 跳过 wire / OTel / JWT / accesslog 等基础设施依赖。
//
// 覆盖：
//   - POST /migrator/tasks 入库 + 异步 audit_log 落表 + DSN 脱敏
//
// 不覆盖：JWT 验签（生产路径由 ioc.InitMiddlewares 装配）/ otelgin / cors / 限流，
// 这些纯透传中间件无需 e2e 重复验证。
type TaskE2ESuite struct {
	suite.Suite
	db     *gorm.DB
	cmd    redis.Cmdable
	server *gin.Engine
}

func TestTaskE2E(t *testing.T) {
	suite.Run(t, &TaskE2ESuite{})
}

func (s *TaskE2ESuite) SetupSuite() {
	// 集成测试在基础设施不可用时自动 skip，避免污染 `go test ./...` 全量回归
	db, err := gorm.Open(mysql.Open(viper.GetString("data.mysql.dsn")))
	if err != nil {
		s.T().Skipf("mysql not available, skipping integration: %v", err)
		return
	}
	if err := dao.InitTable(db); err != nil {
		s.T().Skipf("mysql/migrate not available (build database 'webook_migrator_test' first): %v", err)
		return
	}

	cmd := redis.NewClient(&redis.Options{
		Addr:     viper.GetString("data.redis.addr"),
		Password: viper.GetString("data.redis.password"),
	})
	if err := cmd.Ping(context.Background()).Err(); err != nil {
		s.T().Skipf("redis not available, skipping integration: %v", err)
		return
	}
	s.db = db
	s.cmd = cmd

	l := logger.NewNopLogger()
	taskDAO := dao.NewGormTaskDAO(db)
	auditDAO := dao.NewGormAuditLogDAO(db)

	repo := repository.NewTaskRepository(taskDAO, l)
	svc := service.NewTaskService(repo, nil, l)
	// 引擎类参数传 nil — 集成测试只覆盖 CRUD 路径（POST/GET tasks）+ audit middleware；
	// control endpoint 未调用，nil 安全。
	handler := web.NewTaskHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil)
	audit := middleware.NewAuditMiddleware(repository.NewAuditLogRepository(auditDAO), l)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(audit.Build())
	handler.RegisterRoutes(r)
	s.server = r
}

// TearDownTest 每个用例后清表 + Redis；避免用例间互染。
func (s *TaskE2ESuite) TearDownTest() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE task").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE audit_log").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

func validBody() []byte {
	b, _ := json.Marshal(map[string]any{
		"name":         "e2e_test_v1",
		"mode":         "cdc",
		"kind":         "heterogeneous",
		"sourceDsnRef": "vault:src",
		"sinkType":     "es",
		"sinkDsnRef":   "vault:dst",
		"tables":       []map[string]any{{"src": "article", "dst": "article_v1", "partitionKey": "id"}},
	})
	return b
}

func (s *TaskE2ESuite) TestCreate_Persists_And_AuditLogged() {
	t := s.T()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/migrator/tasks", bytes.NewReader(validBody()))
	req.Header.Set("Content-Type", "application/json")
	s.server.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "create should 200: %s", w.Body.String())

	// task 表入库
	var taskCount int64
	require.NoError(t, s.db.Model(&dao.Task{}).Count(&taskCount).Error)
	assert.EqualValues(t, 1, taskCount, "exactly one task row inserted")

	// audit_log 异步落表，给点时间
	assert.Eventually(t, func() bool {
		var n int64
		_ = s.db.Model(&dao.AuditLog{}).
			Where("action = ?", "create").Count(&n).Error
		return n == 1
	}, 2*time.Second, 50*time.Millisecond, "audit_log should have 1 'create' row")

	// DSN 在 audit_log.payload 中被 mask
	var lg dao.AuditLog
	require.NoError(t, s.db.Model(&dao.AuditLog{}).Where("action = ?", "create").First(&lg).Error)
	assert.NotContains(t, lg.Payload, "vault:src", "audit payload must mask sourceDsnRef")
	assert.NotContains(t, lg.Payload, "vault:dst", "audit payload must mask sinkDsnRef")
	assert.Contains(t, lg.Payload, `"sourceDsnRef":"***"`)
	assert.Contains(t, lg.Payload, `"sinkDsnRef":"***"`)
}
