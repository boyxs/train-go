package job

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	svcmocks "github.com/webook/internal/service/mocks"
	"github.com/webook/pkg/cronx"
	cronprom "github.com/webook/pkg/cronx/prometheus"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/redislockx"
)

// 入口测试：4 个 entry 全部注册成功。Wrap 行为已在 pkg/cronx/wrapper_test.go 覆盖。
func TestRankingJob_RegisterTo_AddsFourEntries(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	svc := svcmocks.NewMockRankingService(ctrl)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	reg := prometheus.NewRegistry()
	metrics := cronprom.NewPrometheusBuilder("webook", "cron", "test").Registry(reg).Build()
	wrapper := cronx.NewWrapper(redislockx.NewClient(rdb), metrics, logger.NewNopLogger())
	j := NewRankingJob(svc, wrapper)

	c := cron.New(cron.WithSeconds())
	err := j.RegisterTo(c)
	assert.NoError(t, err)
	assert.Len(t, c.Entries(), 4)
}
