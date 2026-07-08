package repository

import (
	"context"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/repository/dao"
)

// AuditLogRepository 审计日志仓储（append-only）。
type AuditLogRepository interface {
	Create(ctx context.Context, lg domain.AuditLog) (int64, error)
}

type InternalAuditLogRepository struct {
	dao dao.AuditLogDAO
}

func NewAuditLogRepository(d dao.AuditLogDAO) AuditLogRepository {
	return &InternalAuditLogRepository{dao: d}
}

func (r *InternalAuditLogRepository) Create(ctx context.Context, lg domain.AuditLog) (int64, error) {
	return r.dao.Insert(ctx, r.toEntity(lg))
}

func (r *InternalAuditLogRepository) toEntity(lg domain.AuditLog) dao.AuditLog {
	return dao.AuditLog{
		TaskId:   lg.TaskId,
		Actor:    lg.Actor,
		Action:   lg.Action,
		Payload:  lg.Payload,
		Result:   lg.Result,
		ErrorMsg: lg.ErrorMsg,
		ClientIp: lg.ClientIp,
	}
}
