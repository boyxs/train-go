package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/webook/migrator/errs"
)

// CheckpointUpdate 用于乐观锁更新的入参（封装多个字段，避免长参数列表）。
type CheckpointUpdate struct {
	TaskId          int64
	Phase           string
	ShardNo         int32
	CursorValue     string
	ProgressPercent float64
	LagMs           int64
	ExpectedVersion int64
}

type CheckpointDAO interface {
	// Upsert 插入或按 (task_id, phase, shard_no) 唯一键更新（首次创建）
	Upsert(ctx context.Context, c Checkpoint) (int64, error)

	// FindByTaskAndPhase 查任务在某阶段的所有分片 checkpoint
	FindByTaskAndPhase(ctx context.Context, taskId int64, phase string) ([]Checkpoint, error)

	// UpdateCursor 带乐观锁的游标更新；u.ExpectedVersion 与库中不匹配 → ErrCheckpointVersionConflict
	UpdateCursor(ctx context.Context, u CheckpointUpdate) error

	DeleteByTask(ctx context.Context, taskId int64) error
}

type GormCheckpointDAO struct {
	db *gorm.DB
}

func NewGormCheckpointDAO(db *gorm.DB) CheckpointDAO {
	return &GormCheckpointDAO{db: db}
}

func (d *GormCheckpointDAO) Upsert(ctx context.Context, c Checkpoint) (int64, error) {
	err := d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_id"}, {Name: "phase"}, {Name: "shard_no"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"cursor_kind", "cursor_value", "progress_percent", "last_lag_ms", "updated_at",
		}),
	}).Create(&c).Error
	if err != nil {
		return 0, err
	}
	return c.Id, nil
}

func (d *GormCheckpointDAO) FindByTaskAndPhase(ctx context.Context, taskId int64, phase string) ([]Checkpoint, error) {
	var list []Checkpoint
	err := d.db.WithContext(ctx).
		Where("task_id = ? AND phase = ?", taskId, phase).
		Order("shard_no ASC").
		Find(&list).Error
	return list, err
}

func (d *GormCheckpointDAO) UpdateCursor(ctx context.Context, u CheckpointUpdate) error {
	res := d.db.WithContext(ctx).Model(&Checkpoint{}).
		Where("task_id = ? AND phase = ? AND shard_no = ? AND version = ?",
			u.TaskId, u.Phase, u.ShardNo, u.ExpectedVersion).
		Updates(map[string]any{
			"cursor_value":     u.CursorValue,
			"progress_percent": u.ProgressPercent,
			"last_lag_ms":      u.LagMs,
			"version":          gorm.Expr("version + 1"),
			"updated_at":       time.Now().UnixMilli(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.ErrCheckpointVersionConflict
	}
	return nil
}

func (d *GormCheckpointDAO) DeleteByTask(ctx context.Context, taskId int64) error {
	return d.db.WithContext(ctx).
		Where("task_id = ?", taskId).
		Delete(&Checkpoint{}).Error
}

// ── DAO Model ───────────────────────────────────────────

type Checkpoint struct {
	Id              int64   `gorm:"primaryKey;autoIncrement"`
	TaskId          int64   `gorm:"not null;uniqueIndex:uk_checkpoint_task_phase_shard,priority:1;column:task_id"`
	Phase           string  `gorm:"type:varchar(16);not null;uniqueIndex:uk_checkpoint_task_phase_shard,priority:2"`
	ShardNo         int32   `gorm:"not null;default:0;uniqueIndex:uk_checkpoint_task_phase_shard,priority:3;column:shard_no"`
	CursorKind      string  `gorm:"type:varchar(16);not null;column:cursor_kind"`
	CursorValue     string  `gorm:"type:varchar(256);not null;default:'';column:cursor_value"`
	ProgressPercent float64 `gorm:"type:decimal(5,2);not null;default:0;column:progress_percent"`
	LastLagMs       int64   `gorm:"not null;default:0;column:last_lag_ms"`
	Version         int64   `gorm:"not null;default:0"`
	UpdatedAt       int64   `gorm:"autoUpdateTime:milli;column:updated_at"`
}

func (Checkpoint) TableName() string {
	return "checkpoint"
}
