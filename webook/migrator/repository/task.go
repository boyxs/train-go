package repository

import (
	"context"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/repository/dao"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/slicex"
)

type ListOpts struct {
	Status *domain.TaskStatus
	Offset int
	Limit  int
}

type TaskRepository interface {
	Create(ctx context.Context, t domain.Task) (int64, error)
	FindById(ctx context.Context, id int64) (domain.Task, error)
	List(ctx context.Context, opts ListOpts) ([]domain.Task, int64, error)
	// UpdateStatus 推进 task.status — 引擎/Switch 服务在状态变化时调。
	UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus) error
	// UpdateGrayPercent 同步灰度比例到 task 表冗余列（Redis 才是路由决策源）。
	UpdateGrayPercent(ctx context.Context, id int64, percent int16) error
}

type InternalTaskRepository struct {
	dao dao.TaskDAO
	l   logger.LoggerX
}

// NewTaskRepository 构造 TaskRepository。
// 审计落表由 web/middleware/audit.go 统一负责（cross-cutting concern），不在 repo 层重复实现。
func NewTaskRepository(d dao.TaskDAO, l logger.LoggerX) TaskRepository {
	return &InternalTaskRepository{dao: d, l: l}
}

func (r *InternalTaskRepository) Create(ctx context.Context, t domain.Task) (int64, error) {
	m := r.toEntity(t)
	return r.dao.Insert(ctx, m)
}

func (r *InternalTaskRepository) FindById(ctx context.Context, id int64) (domain.Task, error) {
	m, err := r.dao.FindById(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(m), nil
}

func (r *InternalTaskRepository) UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus) error {
	return r.dao.UpdateStatus(ctx, id, int8(status))
}

func (r *InternalTaskRepository) UpdateGrayPercent(ctx context.Context, id int64, percent int16) error {
	return r.dao.UpdateGrayPercent(ctx, id, percent)
}

func (r *InternalTaskRepository) List(ctx context.Context, opts ListOpts) ([]domain.Task, int64, error) {
	var statusPtr *int8
	if opts.Status != nil {
		v := int8(*opts.Status)
		statusPtr = &v
	}
	list, total, err := r.dao.List(ctx, statusPtr, opts.Offset, opts.Limit)
	if err != nil {
		return nil, 0, err
	}
	return slicex.Map(list, r.toDomain), total, nil
}

func (r *InternalTaskRepository) toEntity(t domain.Task) dao.Task {
	return dao.Task{
		Id:           t.Id,
		Name:         t.Name,
		Mode:         string(t.Mode),
		Kind:         string(t.Kind),
		SourceType:   string(t.SourceType),
		SourceDsnRef: t.SourceDsnRef,
		SinkType:     t.SinkType,
		SinkDsnRef:   t.SinkDsnRef,
		TablesJSON:   t.TablesJSON,
		Status:       int8(t.Status),
		GrayPercent:  t.GrayPercent,
		Consistency:  t.Consistency,
	}
}

func (r *InternalTaskRepository) toDomain(m dao.Task) domain.Task {
	return domain.Task{
		Id:           m.Id,
		Name:         m.Name,
		Mode:         domain.Mode(m.Mode),
		Kind:         domain.Kind(m.Kind),
		SourceType:   domain.SourceType(m.SourceType),
		SourceDsnRef: m.SourceDsnRef,
		SinkType:     m.SinkType,
		SinkDsnRef:   m.SinkDsnRef,
		TablesJSON:   m.TablesJSON,
		Status:       domain.TaskStatus(m.Status),
		GrayPercent:  m.GrayPercent,
		Consistency:  m.Consistency,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
