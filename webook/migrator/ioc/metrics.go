package ioc

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/service/incr"
	"github.com/webook/pkg/logger"
)

// 指标命名遵守根 CLAUDE.md：webook_<subsystem>_*（subsystem=migration），禁止 webook_<service>_*；
// service 维度由 prometheus 抓取自动注入的 job label（webook-migrator）区分。
var (
	lagDesc = prometheus.NewDesc(
		"webook_migration_lag_ms",
		"增量同步延迟（毫秒）。side=src：消费到的 binlog 事件时间→now；side=dst：最后成功写 dst 的事件时间→now。仅运行中任务有样本。",
		[]string{"task_id", "side"}, nil)
	dlDesc = prometheus.NewDesc(
		"webook_migration_dead_letter_unreplayed",
		"未重放死信行数（replayed=0 且 replay_failed=0），按 task 聚合。",
		[]string{"task_id"}, nil)
)

// MigrationMetricsCollector scrape 时实时采集迁移业务指标：
//   - lag：枚举 IncrEngine 运行中任务逐个取 Lag/LagDst（任务不跑 = 无样本，面板自然 No data）
//   - dead letter：控制库 GROUP BY 聚合（表小、scrape 间隔 15s，一条 COUNT 查询可接受）
type MigrationMetricsCollector struct {
	incrEng incr.IncrEngine
	dlRepo  repository.DeadLetterRepository
	l       logger.LoggerX
}

// InitMigrationMetrics 构造并注册 Collector 到默认 registry（/metrics 即 promhttp 默认 handler）。
func InitMigrationMetrics(ie incr.IncrEngine, dlRepo repository.DeadLetterRepository, l logger.LoggerX) *MigrationMetricsCollector {
	c := &MigrationMetricsCollector{incrEng: ie, dlRepo: dlRepo, l: l}
	prometheus.MustRegister(c)
	return c
}

func (c *MigrationMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- lagDesc
	ch <- dlDesc
}

func (c *MigrationMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	for _, id := range c.incrEng.RunningTasks() {
		tid := strconv.FormatInt(id, 10)
		if v, err := c.incrEng.Lag(id); err == nil {
			ch <- prometheus.MustNewConstMetric(lagDesc, prometheus.GaugeValue, float64(v), tid, "src")
		}
		if v, err := c.incrEng.LagDst(id); err == nil && v >= 0 {
			ch <- prometheus.MustNewConstMetric(lagDesc, prometheus.GaugeValue, float64(v), tid, "dst")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	counts, err := c.dlRepo.CountUnreplayedByTask(ctx)
	if err != nil {
		c.l.Warn("collect dead_letter counts failed", logger.Error(err))
		return
	}
	for id, n := range counts {
		ch <- prometheus.MustNewConstMetric(dlDesc, prometheus.GaugeValue, float64(n), strconv.FormatInt(id, 10))
	}
}
