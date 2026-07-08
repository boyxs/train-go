package job

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	rankingv1 "github.com/boyxs/train-go/webook/api/gen/ranking/v1"
	"github.com/boyxs/train-go/webook/pkg/cronx"
)

const (
	specHot     = "0 */1 * * * *"
	specBest    = "0 */5 * * * *"
	specNew     = "*/30 * * * * *"
	specArchive = "0 10 0 * * *"

	jobTimeout     = 55 * time.Second
	archiveTimeout = 2 * time.Minute
)

// RankingJob 调度器侧榜单定时任务：只按 cron 触发，重算/归档逻辑在 core（经 RankingJobService gRPC）。
// 锁 / 指标 / date 注入 / panic recover 模板由 *cronx.Wrapper 统一处理；本 Job 只声明 spec→fn 表 + 派发 gRPC。
type RankingJob struct {
	client  rankingv1.RankingJobServiceClient
	wrapper *cronx.Wrapper
}

func NewRankingJob(client rankingv1.RankingJobServiceClient, w *cronx.Wrapper) *RankingJob {
	return &RankingJob{client: client, wrapper: w}
}

// RegisterTo 把所有榜单任务注册到 cron。任一 AddFunc 失败立即返回。
// task 名与 core 时代一致（指标/告警按 task 标签匹配，不可改）。
func (j *RankingJob) RegisterTo(c *cron.Cron) error {
	entries := []struct {
		spec    string
		name    string
		timeout time.Duration
		fn      cronx.Task
	}{
		{specHot, "ranking_hot_recompute", jobTimeout, j.recompute(rankingv1.Dimension_DIMENSION_HOT)},
		{specBest, "ranking_best_recompute", jobTimeout, j.recompute(rankingv1.Dimension_DIMENSION_BEST)},
		{specNew, "ranking_new_recompute", jobTimeout, j.recompute(rankingv1.Dimension_DIMENSION_NEW)},
		{specArchive, "ranking_archive", archiveTimeout, j.archive},
	}
	for _, e := range entries {
		if _, err := c.AddFunc(e.spec, j.wrapper.Wrap(e.name, e.timeout, e.fn)); err != nil {
			return fmt.Errorf("注册 %s 失败: %w", e.name, err)
		}
	}
	return nil
}

// recompute 返回触发某维度榜单重算的 cronx.Task；date/ctx 由 wrapper 注入。
func (j *RankingJob) recompute(dim rankingv1.Dimension) cronx.Task {
	return func(ctx context.Context, date string) error {
		_, err := j.client.Recompute(ctx, &rankingv1.RecomputeRequest{Dimension: dim, Date: date})
		return err
	}
}

func (j *RankingJob) archive(ctx context.Context, date string) error {
	_, err := j.client.Archive(ctx, &rankingv1.ArchiveRequest{Date: date})
	return err
}
