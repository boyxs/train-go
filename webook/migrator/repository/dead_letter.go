package repository

import (
	"context"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/repository/dao"
	"github.com/webook/pkg/slicex"
)

// DeadLetterRepository 死信仓储（重放消费侧用到的子集；落死信的生产侧直接走 Sink 链路）。
type DeadLetterRepository interface {
	// ListUnreplayedByTask 取待重放的死信（created_at ASC，最老优先）。
	ListUnreplayedByTask(ctx context.Context, taskId int64, limit int) ([]domain.DeadLetter, error)
	// MarkReplayed 批量标记重放成功。
	MarkReplayed(ctx context.Context, ids []int64) error
	// IncrementRetry 一次重放失败：retry_count++ + 记 last_error。
	IncrementRetry(ctx context.Context, id int64, lastErr string) error
	// CountUnreplayedByTask 按 task 聚合未重放死信行数（监控采集用）。
	CountUnreplayedByTask(ctx context.Context) (map[int64]int64, error)
}

type InternalDeadLetterRepository struct {
	dao dao.DeadLetterDAO
}

func NewDeadLetterRepository(d dao.DeadLetterDAO) DeadLetterRepository {
	return &InternalDeadLetterRepository{dao: d}
}

func (r *InternalDeadLetterRepository) ListUnreplayedByTask(ctx context.Context, taskId int64, limit int) ([]domain.DeadLetter, error) {
	rows, err := r.dao.ListUnreplayedByTask(ctx, taskId, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(rows, r.toDomain), nil
}

func (r *InternalDeadLetterRepository) toDomain(m dao.DeadLetter) domain.DeadLetter {
	return domain.DeadLetter{
		Id:           m.Id,
		TaskId:       m.TaskId,
		Op:           m.Op,
		BizTable:     m.BizTable,
		BizId:        m.BizId,
		Payload:      m.Payload,
		LastError:    m.LastError,
		RetryCount:   m.RetryCount,
		Replayed:     m.Replayed,
		ReplayFailed: m.ReplayFailed,
		CreatedAt:    m.CreatedAt,
		ReplayedAt:   m.ReplayedAt,
	}
}

func (r *InternalDeadLetterRepository) MarkReplayed(ctx context.Context, ids []int64) error {
	return r.dao.MarkReplayed(ctx, ids)
}

func (r *InternalDeadLetterRepository) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	return r.dao.IncrementRetry(ctx, id, lastErr)
}

func (r *InternalDeadLetterRepository) CountUnreplayedByTask(ctx context.Context) (map[int64]int64, error) {
	return r.dao.CountUnreplayedByTask(ctx)
}
