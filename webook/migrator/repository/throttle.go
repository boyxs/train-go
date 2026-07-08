package repository

import (
	"context"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/repository/cache"
)

// ThrottleRepository task 级限速配置仓储（Redis 持久化，"下次 Start 生效"语义）。
type ThrottleRepository interface {
	Save(ctx context.Context, taskId int64, cfg domain.ThrottleConfig) error
	// Find 未设置返回 (zero, false, nil)。
	Find(ctx context.Context, taskId int64) (domain.ThrottleConfig, bool, error)
	Clear(ctx context.Context, taskId int64) error
}

type CacheThrottleRepository struct {
	cache cache.ThrottleCache
}

func NewThrottleRepository(c cache.ThrottleCache) ThrottleRepository {
	return &CacheThrottleRepository{cache: c}
}

func (r *CacheThrottleRepository) Save(ctx context.Context, taskId int64, cfg domain.ThrottleConfig) error {
	return r.cache.Set(ctx, taskId, cache.ThrottleConfig{QPS: cfg.QPS, ShardWorkers: cfg.ShardWorkers})
}

func (r *CacheThrottleRepository) Find(ctx context.Context, taskId int64) (domain.ThrottleConfig, bool, error) {
	cfg, ok, err := r.cache.Get(ctx, taskId)
	if err != nil || !ok {
		return domain.ThrottleConfig{}, false, err
	}
	return domain.ThrottleConfig{QPS: cfg.QPS, ShardWorkers: cfg.ShardWorkers}, true, nil
}

func (r *CacheThrottleRepository) Clear(ctx context.Context, taskId int64) error {
	return r.cache.Clear(ctx, taskId)
}
