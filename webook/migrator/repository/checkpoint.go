package repository

import (
	"context"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/repository/dao"
	"github.com/webook/pkg/slicex"
)

// CheckpointRepository checkpoint 仓储。引擎层经此持久化/恢复游标，不直接触 DAO。
type CheckpointRepository interface {
	// Save 插入或按 (task_id, phase, shard_no) 唯一键更新。
	Save(ctx context.Context, c domain.Checkpoint) error
	// ListByTaskPhase 查任务在某阶段的全部分片 checkpoint（shard_no ASC）。
	ListByTaskPhase(ctx context.Context, taskId int64, phase string) ([]domain.Checkpoint, error)
}

type InternalCheckpointRepository struct {
	dao dao.CheckpointDAO
}

func NewCheckpointRepository(d dao.CheckpointDAO) CheckpointRepository {
	return &InternalCheckpointRepository{dao: d}
}

func (r *InternalCheckpointRepository) Save(ctx context.Context, c domain.Checkpoint) error {
	_, err := r.dao.Upsert(ctx, r.toEntity(c))
	return err
}

func (r *InternalCheckpointRepository) ListByTaskPhase(ctx context.Context, taskId int64, phase string) ([]domain.Checkpoint, error) {
	list, err := r.dao.FindByTaskAndPhase(ctx, taskId, phase)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomain), nil
}

func (r *InternalCheckpointRepository) toEntity(c domain.Checkpoint) dao.Checkpoint {
	return dao.Checkpoint{
		TaskId:          c.TaskId,
		Phase:           c.Phase,
		ShardNo:         c.ShardNo,
		CursorKind:      c.CursorKind,
		CursorValue:     c.CursorValue,
		ProgressPercent: c.ProgressPercent,
		LastLagMs:       c.LastLagMs,
		Version:         c.Version,
	}
}

func (r *InternalCheckpointRepository) toDomain(m dao.Checkpoint) domain.Checkpoint {
	return domain.Checkpoint{
		TaskId:          m.TaskId,
		Phase:           m.Phase,
		ShardNo:         m.ShardNo,
		CursorKind:      m.CursorKind,
		CursorValue:     m.CursorValue,
		ProgressPercent: m.ProgressPercent,
		LastLagMs:       m.LastLagMs,
		Version:         m.Version,
	}
}
