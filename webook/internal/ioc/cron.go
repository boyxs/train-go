package ioc

import (
	"github.com/robfig/cron/v3"

	"github.com/webook/internal/job"
	"github.com/webook/pkg/cronx"
	cronprom "github.com/webook/pkg/cronx/prometheus"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/redislockx"
)

// InitCron 创建 cron.Cron + 注册 Job + 启动；返回 cleanup 在 SIGTERM 时等 in-flight 跑完。
func InitCron(rankingJob *job.RankingJob, l logger.LoggerX) (*cron.Cron, func()) {
	c := cron.New(cron.WithSeconds())
	if err := rankingJob.RegisterTo(c); err != nil {
		panic(err)
	}
	c.Start()
	l.Info("cron 已启动")

	cleanup := func() {
		l.Info("cron 收到关停信号，等待 in-flight 任务完成…")
		<-c.Stop().Done()
		l.Info("cron 已停止")
	}
	return c, cleanup
}

// InitCronMetrics 任务级指标统一在这一处注册。所有 Job 共享 *Metrics 句柄，task 标签区分。
func InitCronMetrics() *cronprom.Metrics {
	return cronprom.NewPrometheusBuilder("webook_core", "cron", "定时任务").Build()
}

// InitCronWrapper 把锁/指标/日志组装成 *cronx.Wrapper，所有 Job 复用。
func InitCronWrapper(lock redislockx.Client, m *cronprom.Metrics, l logger.LoggerX) *cronx.Wrapper {
	return cronx.NewWrapper(lock, m, l)
}
