package repository

import (
	"context"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/repository/cache"
)

// SwitchStateRepository 切流状态仓储（stage / gray / cutover propose）。
// 状态存 Redis（SDK 路由决策真相源），本仓储是 cache 的领域语义封装；
// task 表的 gray_percent 冗余列由 TaskRepository.UpdateGrayPercent 单独维护。
type SwitchStateRepository interface {
	SetGray(ctx context.Context, taskName string, percent int) error
	// GetGray 未设置返回 0。
	GetGray(ctx context.Context, taskName string) (int, error)
	SetStage(ctx context.Context, taskName string, stage domain.Stage) error
	// GetStage 未设置返回空 Stage("")；默认语义（SRC_ONLY）由 service 解释。
	GetStage(ctx context.Context, taskName string) (domain.Stage, error)
	// SavePropose 注册 cutover propose actor（带 TTL，过期自动失效）。
	SavePropose(ctx context.Context, taskName, actor string) error
	// FindPropose 未设置（或已过期）返回 ""。
	FindPropose(ctx context.Context, taskName string) (string, error)
	DeletePropose(ctx context.Context, taskName string) error
}

type CacheSwitchStateRepository struct {
	cache cache.SwitchStateCache
}

func NewSwitchStateRepository(c cache.SwitchStateCache) SwitchStateRepository {
	return &CacheSwitchStateRepository{cache: c}
}

func (r *CacheSwitchStateRepository) SetGray(ctx context.Context, taskName string, percent int) error {
	return r.cache.SetGray(ctx, taskName, percent)
}

func (r *CacheSwitchStateRepository) GetGray(ctx context.Context, taskName string) (int, error) {
	return r.cache.GetGray(ctx, taskName)
}

func (r *CacheSwitchStateRepository) SetStage(ctx context.Context, taskName string, stage domain.Stage) error {
	return r.cache.SetStage(ctx, taskName, string(stage))
}

func (r *CacheSwitchStateRepository) GetStage(ctx context.Context, taskName string) (domain.Stage, error) {
	v, err := r.cache.GetStage(ctx, taskName)
	if err != nil {
		return "", err
	}
	return domain.Stage(v), nil
}

func (r *CacheSwitchStateRepository) SavePropose(ctx context.Context, taskName, actor string) error {
	return r.cache.SetPropose(ctx, taskName, actor)
}

func (r *CacheSwitchStateRepository) FindPropose(ctx context.Context, taskName string) (string, error) {
	return r.cache.GetPropose(ctx, taskName)
}

func (r *CacheSwitchStateRepository) DeletePropose(ctx context.Context, taskName string) error {
	return r.cache.DelPropose(ctx, taskName)
}
