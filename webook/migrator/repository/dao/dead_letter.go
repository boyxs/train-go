package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type DeadLetterDAO interface {
	Insert(ctx context.Context, dl DeadLetter) (int64, error)
	// ListUnreplayedByTask 取待重放的死信（最老优先）
	ListUnreplayedByTask(ctx context.Context, taskId int64, limit int) ([]DeadLetter, error)
	// IncrementRetry 一次重放失败：retry_count++ + 记 last_error
	IncrementRetry(ctx context.Context, id int64, lastError string) error
	// MarkReplayed 重放成功
	MarkReplayed(ctx context.Context, ids []int64) error
	// MarkReplayFailed 多次重放仍失败 → 标记为人工处理
	MarkReplayFailed(ctx context.Context, ids []int64, lastError string) error
	// CountUnreplayedByTask 按 task 聚合未重放死信行数（监控采集用）
	CountUnreplayedByTask(ctx context.Context) (map[int64]int64, error)
}

type GormDeadLetterDAO struct {
	db *gorm.DB
}

func NewGormDeadLetterDAO(db *gorm.DB) DeadLetterDAO {
	return &GormDeadLetterDAO{db: db}
}

func (d *GormDeadLetterDAO) Insert(ctx context.Context, dl DeadLetter) (int64, error) {
	if err := d.db.WithContext(ctx).Create(&dl).Error; err != nil {
		return 0, err
	}
	return dl.Id, nil
}

func (d *GormDeadLetterDAO) ListUnreplayedByTask(ctx context.Context, taskId int64, limit int) ([]DeadLetter, error) {
	var list []DeadLetter
	err := d.db.WithContext(ctx).
		Where("task_id = ? AND replayed = 0 AND replay_failed = 0", taskId).
		Order("created_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (d *GormDeadLetterDAO) IncrementRetry(ctx context.Context, id int64, lastError string) error {
	return d.db.WithContext(ctx).Model(&DeadLetter{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"retry_count": gorm.Expr("retry_count + 1"),
			"last_error":  lastError,
		}).Error
}

func (d *GormDeadLetterDAO) MarkReplayed(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Model(&DeadLetter{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"replayed":    1,
			"replayed_at": time.Now().UnixMilli(),
		}).Error
}

func (d *GormDeadLetterDAO) MarkReplayFailed(ctx context.Context, ids []int64, lastError string) error {
	if len(ids) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Model(&DeadLetter{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"replay_failed": 1,
			"last_error":    lastError,
		}).Error
}

func (d *GormDeadLetterDAO) CountUnreplayedByTask(ctx context.Context) (map[int64]int64, error) {
	type row struct {
		TaskId int64
		N      int64
	}
	var rows []row
	err := d.db.WithContext(ctx).Model(&DeadLetter{}).
		Select("task_id, COUNT(*) AS n").
		Where("replayed = 0 AND replay_failed = 0").
		Group("task_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int64, len(rows))
	for _, r := range rows {
		out[r.TaskId] = r.N
	}
	return out, nil
}

// ── DAO Model ───────────────────────────────────────────

type DeadLetter struct {
	Id     int64  `gorm:"primaryKey;autoIncrement"`
	TaskId int64  `gorm:"not null;column:task_id;index:idx_dead_letter_task_replayed,priority:1"`
	Op     string `gorm:"type:varchar(16);not null"`
	// BizTable 源表名（task.tables[].src），非目标表名 —— ReplayDL 按它反查 tableIdx；
	// 目标表名由 BuildDst 出来的 sink 自己持有，Mutation.Table 不参与 MySQLSink 写入。
	BizTable     string `gorm:"type:varchar(64);not null;column:table_name;index:idx_dead_letter_table_biz,priority:1"`
	BizId        string `gorm:"type:varchar(64);not null;column:biz_id;index:idx_dead_letter_table_biz,priority:2"`
	Payload      string `gorm:"type:text;not null"`
	LastError    string `gorm:"type:varchar(1024);column:last_error"`
	RetryCount   int    `gorm:"not null;default:0;column:retry_count"`
	Replayed     int8   `gorm:"not null;default:0;index:idx_dead_letter_task_replayed,priority:2"`
	ReplayFailed int8   `gorm:"not null;default:0;column:replay_failed"`
	CreatedAt    int64  `gorm:"autoCreateTime:milli;column:created_at;index:idx_dead_letter_created"`
	ReplayedAt   int64  `gorm:"not null;default:0;column:replayed_at"`
}

func (DeadLetter) TableName() string {
	return "dead_letter"
}
