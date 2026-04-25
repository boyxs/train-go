package job

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-module/carbon/v2"
	"github.com/robfig/cron/v3"

	"github.com/webook/internal/service"
	"github.com/webook/pkg/logger"
)

// 测试节奏 vs 生产节奏：本地开发用短间隔；上线前改回生产节奏
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
// 把 cron 表达式 + 业务回调 + panic recover 封装在一起，
// ioc 只负责生命周期（新建 cron.Cron + 启动 + 关停），不认识具体任务。
type RankingJob struct {
	svc service.RankingService
	l   logger.LoggerX
}

func NewRankingJob(svc service.RankingService, l logger.LoggerX) *RankingJob {
	return &RankingJob{svc: svc, l: l}
}

// RegisterTo 把所有榜单任务注册到 cron。任一 AddFunc 失败立即返回。
func (j *RankingJob) RegisterTo(c *cron.Cron) error {
	type entry struct {
		spec    string
		name    string
		timeout time.Duration
		fn      func(ctx context.Context, date string) error
	}
	entries := []entry{
		{specHot, "hot_recompute", jobTimeout, j.svc.RecomputeHot},
		{specBest, "best_recompute", jobTimeout, j.svc.RecomputeBest},
		{specNew, "new_recompute", jobTimeout, j.svc.RecomputeNew},
		{specArchive, "archive", archiveTimeout, j.svc.Archive},
	}
	for _, e := range entries {
		if _, err := c.AddFunc(e.spec, j.wrap(e.name, e.timeout, e.fn)); err != nil {
			return fmt.Errorf("注册 %s 失败: %w", e.name, err)
		}
	}
	j.l.Info("ranking 任务已注册", logger.Int("count", len(entries)))
	return nil
}

// wrap 把 (name, timeout, fn) 包装为无参 cron callback。
// 负责 ctx / 超时 / panic recover / 日志，所有 Job 共用，避免散写重复。
func (j *RankingJob) wrap(name string, timeout time.Duration, fn func(ctx context.Context, date string) error) func() {
	return func() {
		defer j.recoverOnPanic(name)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		date := carbon.Now().ToDateString()
		if err := fn(ctx, date); err != nil {
			j.l.Error("ranking 任务失败",
				logger.String("task", name),
				logger.String("date", date),
				logger.Error(err))
		}
	}
}

func (j *RankingJob) recoverOnPanic(name string) {
	r := recover()
	if r == nil {
		return
	}
	j.l.Error("ranking 任务 panic",
		logger.String("task", name),
		logger.String("panic", formatAny(r)))
}

func formatAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return fmt.Sprintf("%v", x)
	}
}
