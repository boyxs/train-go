package job

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/webook/internal/service"
	"github.com/webook/pkg/cronx"
)

const (
	//specHot     = "*/10 * * * * *" // 测试每 10s；生产 "0 */1 * * * *"
	//specBest    = "*/15 * * * * *" // 测试每 15s；生产 "0 */5 * * * *"
	//specNew     = "*/5 * * * * *"  // 测试每 5s ；生产 "*/30 * * * * *"
	//specArchive = "0 */10 * * * *" // 测试每 10min；生产 "0 10 0 * * *"
	specHot     = "0 */1 * * * *"
	specBest    = "0 */5 * * * *"
	specNew     = "*/30 * * * * *"
	specArchive = "0 10 0 * * *"

	jobTimeout     = 55 * time.Second
	archiveTimeout = 2 * time.Minute
)

// RankingJob 榜单相关的定时任务集合。
// 锁 / 指标 / panic 模板由 *cronx.Wrapper 统一处理；本 Job 只负责声明 spec→fn 表。
type RankingJob struct {
	svc     service.RankingService
	wrapper *cronx.Wrapper
}

func NewRankingJob(svc service.RankingService, w *cronx.Wrapper) *RankingJob {
	return &RankingJob{svc: svc, wrapper: w}
}

// RegisterTo 把所有榜单任务注册到 cron。任一 AddFunc 失败立即返回。
func (j *RankingJob) RegisterTo(c *cron.Cron) error {
	type entry struct {
		spec    string
		name    string
		timeout time.Duration
		fn      cronx.Task
	}
	entries := []entry{
		{specHot, "ranking_hot_recompute", jobTimeout, j.svc.RecomputeHot},
		{specBest, "ranking_best_recompute", jobTimeout, j.svc.RecomputeBest},
		{specNew, "ranking_new_recompute", jobTimeout, j.svc.RecomputeNew},
		{specArchive, "ranking_archive", archiveTimeout, j.svc.Archive},
	}
	for _, e := range entries {
		if _, err := c.AddFunc(e.spec, j.wrapper.Wrap(e.name, e.timeout, e.fn)); err != nil {
			return fmt.Errorf("注册 %s 失败: %w", e.name, err)
		}
	}
	return nil
}
