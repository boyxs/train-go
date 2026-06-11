package dao

import (
	"context"

	"gorm.io/gorm"
)

type AuditLogDAO interface {
	// Insert append-only 审计记录
	Insert(ctx context.Context, log AuditLog) (int64, error)
}

type GormAuditLogDAO struct {
	db *gorm.DB
}

func NewGormAuditLogDAO(db *gorm.DB) AuditLogDAO {
	return &GormAuditLogDAO{db: db}
}

func (d *GormAuditLogDAO) Insert(ctx context.Context, log AuditLog) (int64, error) {
	if err := d.db.WithContext(ctx).Create(&log).Error; err != nil {
		return 0, err
	}
	return log.Id, nil
}

// ── DAO Model ───────────────────────────────────────────

type AuditLog struct {
	Id        int64  `gorm:"primaryKey;autoIncrement"`
	TaskId    int64  `gorm:"not null;column:task_id;index:idx_audit_log_task_created,priority:1"`
	Actor     string `gorm:"type:varchar(64);not null;index:idx_audit_log_actor,priority:1"`
	Action    string `gorm:"type:varchar(32);not null"`
	Payload   string `gorm:"type:text"`
	Result    string `gorm:"type:varchar(16);not null"`
	ErrorMsg  string `gorm:"type:varchar(512);column:error_msg"`
	ClientIp  string `gorm:"type:varchar(64);column:client_ip"`
	CreatedAt int64  `gorm:"autoCreateTime:milli;column:created_at;index:idx_audit_log_task_created,priority:2;index:idx_audit_log_actor,priority:2"`
}

func (AuditLog) TableName() string {
	return "audit_log"
}
