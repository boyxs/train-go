package ioc

import (
	"time"

	"github.com/robfig/cron/v3"

	"github.com/boyxs/train-go/webook/pkg/cronx"
	cronprom "github.com/boyxs/train-go/webook/pkg/cronx/prometheus"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/redislockx"
	"github.com/boyxs/train-go/webook/worker/job"
)

// InitCronMetrics 任务级指标统一在此注册，所有 Job 共享 *Metrics 句柄，task 标签区分（webook_cron_*）。
func InitCronMetrics() *cronprom.Metrics {
	return cronprom.NewPrometheusBuilder("webook", "cron", "定时任务").Build()
}

// InitCronWrapper 组装锁/指标/日志为 *cronx.Wrapper，所有 Job 复用。
// 显式 WithLockTTL(30s)：榜单任务分钟级（重算/归档 ~1-2min），30s 是实例 crash 后的让贤窗口，
// watchdog 每 10s（ttl/3）续约保活；不显式声明就吃 cronx 默认，多副本单跑保证不能依赖隐式默认值。
// 丢锁由 prometheus 装饰器记 webook_lock_watchdog_lost_total（勿在此再传 WithOnLost，会覆盖该指标）。
func InitCronWrapper(lock redislockx.Client, m *cronprom.Metrics, l logger.LoggerX) *cronx.Wrapper {
	return cronx.NewWrapper(lock, m, l, cronx.WithLockTTL(30*time.Second))
}

// InitCron 构造 cron 并注册榜单任务，只构造不 Start（启停归 main），panic recover 已由 wrapper 处理。
// 入参 TimezoneReady 保证时区先于 wrapper 算 date。
func InitCron(_ TimezoneReady, rankingJob *job.RankingJob) (*cron.Cron, error) {
	c := cron.New(cron.WithSeconds())
	if err := rankingJob.RegisterTo(c); err != nil {
		return nil, err
	}
	return c, nil
}
