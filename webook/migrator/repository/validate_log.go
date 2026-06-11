package repository

import (
	"context"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/repository/dao"
	"github.com/webook/pkg/slicex"
)

// ValidateLogRepository 对账差异记录仓储（validate_log 表）。
type ValidateLogRepository interface {
	// BatchInsert 批量落差异；(task_id, table_name, biz_id) 冲突时更新差异详情并重置 repaired=0
	// （"差异仍存在 → 重新提醒"语义，见 DAO BatchInsert 注释）。
	BatchInsert(ctx context.Context, logs []domain.ValidateLog) error
	// ListUnrepaired 列未修复差异，created_at ASC（最老优先）。
	ListUnrepaired(ctx context.Context, taskId int64, offset, limit int) ([]domain.ValidateLog, int64, error)
	// FindByIDs 按 id 批量拉取（Repair overwrite 取 diff_detail 用）。
	FindByIDs(ctx context.Context, ids []int64) ([]domain.ValidateLog, error)
	// MarkRepaired 批量标记已修复。
	MarkRepaired(ctx context.Context, ids []int64) error
}

type InternalValidateLogRepository struct {
	dao dao.ValidateLogDAO
}

func NewValidateLogRepository(d dao.ValidateLogDAO) ValidateLogRepository {
	return &InternalValidateLogRepository{dao: d}
}

func (r *InternalValidateLogRepository) BatchInsert(ctx context.Context, logs []domain.ValidateLog) error {
	return r.dao.BatchInsert(ctx, slicex.Map(logs, r.toEntity))
}

func (r *InternalValidateLogRepository) ListUnrepaired(ctx context.Context, taskId int64, offset, limit int) ([]domain.ValidateLog, int64, error) {
	rows, total, err := r.dao.ListUnrepaired(ctx, taskId, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	return slicex.Map(rows, r.toDomain), total, nil
}

func (r *InternalValidateLogRepository) FindByIDs(ctx context.Context, ids []int64) ([]domain.ValidateLog, error) {
	rows, err := r.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return slicex.Map(rows, r.toDomain), nil
}

func (r *InternalValidateLogRepository) MarkRepaired(ctx context.Context, ids []int64) error {
	return r.dao.MarkRepaired(ctx, ids)
}

func (r *InternalValidateLogRepository) toEntity(lg domain.ValidateLog) dao.ValidateLog {
	return dao.ValidateLog{
		Id:           lg.Id,
		TaskId:       lg.TaskId,
		Direction:    lg.Direction,
		BizTable:     lg.BizTable,
		BizId:        lg.BizId,
		MismatchKind: lg.MismatchKind,
		DiffDetail:   lg.DiffDetail,
		Repaired:     lg.Repaired,
		CreatedAt:    lg.CreatedAt,
		RepairedAt:   lg.RepairedAt,
	}
}

func (r *InternalValidateLogRepository) toDomain(m dao.ValidateLog) domain.ValidateLog {
	return domain.ValidateLog{
		Id:           m.Id,
		TaskId:       m.TaskId,
		Direction:    m.Direction,
		BizTable:     m.BizTable,
		BizId:        m.BizId,
		MismatchKind: m.MismatchKind,
		DiffDetail:   m.DiffDetail,
		Repaired:     m.Repaired,
		CreatedAt:    m.CreatedAt,
		RepairedAt:   m.RepairedAt,
	}
}
