package ioc

import (
	"github.com/robfig/cron/v3"

	"github.com/webook/internal/job"
	"github.com/webook/pkg/logger"
)

// InitCron 只负责 cron.Cron 生命周期：创建、挂载 job、启动。
// 不认识具体业务任务 —— 新增 Job 只要在参数上加一个 *job.XxxJob 并 RegisterTo。
func InitCron(rankingJob *job.RankingJob, l logger.LoggerX) *cron.Cron {
	c := cron.New(cron.WithSeconds())
	if err := rankingJob.RegisterTo(c); err != nil {
		panic(err)
	}
	c.Start()
	l.Info("cron 已启动")
	return c
}
