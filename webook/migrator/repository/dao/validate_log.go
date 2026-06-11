package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ValidateLogDAO interface {
	Insert(ctx context.Context, log ValidateLog) (int64, error)
	BatchInsert(ctx context.Context, logs []ValidateLog) error
	// ListUnrepaired 列未修复的 mismatch，按 created_at ASC（最老优先处理）
	ListUnrepaired(ctx context.Context, taskId int64, offset, limit int) ([]ValidateLog, int64, error)
	// FindByIDs 按 id 批量拉取 — Repair overwrite 拿 diff_detail 用
	FindByIDs(ctx context.Context, ids []int64) ([]ValidateLog, error)
	// MarkRepaired 批量标记已修复
	MarkRepaired(ctx context.Context, ids []int64) error
}

type GormValidateLogDAO struct {
	db *gorm.DB
}

func NewGormValidateLogDAO(db *gorm.DB) ValidateLogDAO {
	return &GormValidateLogDAO{db: db}
}

func (d *GormValidateLogDAO) Insert(ctx context.Context, log ValidateLog) (int64, error) {
	if err := d.db.WithContext(ctx).Create(&log).Error; err != nil {
		return 0, err
	}
	return log.Id, nil
}

// BatchInsert 批量写入对账差异，按唯一索引 uk_validate_log_dedup (task_id, table_name, biz_id)
// 执行 upsert：冲突行更新 mismatch_kind / diff_detail / created_at，并强制重置 repaired=0；
// repaired_at 不覆盖，保留上一次 mark 的时间戳作审计参考。
//
// 重置 repaired 的语义：差异在新一轮对账中再次出现，说明尚未真正修复，应重新进入未修复列表；
// 否则 mark_only 之后同一差异永久脱离视野，verify→mark 即形成漏修缺口（曾实际发生）。
//
// repaired 必须以 clause.Assignments 绑定字面值（生成 repaired=?），不可用 AssignmentColumns
// （生成 repaired=VALUES(repaired)，取值依赖 INSERT 列清单）。GORM 对带 default tag 的字段
// 存在两类隐式行为，均会使 VALUES() 的取值偏离结构体字段值：
//  1. 默认值可解析（如 int 配 default:0）：INSERT 时零值被替换为解析出的默认值并回写结构体；
//  2. 默认值由数据库生成（default:(0) / default:null / sql.NullX 等，DefaultValueInterface 为 nil）：
//     零值字段不进入 INSERT 列清单，VALUES(repaired) 引用悬空，该列回落为隐式值或 NULL。
//
// 字面值绑定与 INSERT 列清单解耦，对两类行为均不敏感。
//
// 源码索引（gorm.io/gorm，v1.20 起逻辑一致）：
//
//	schema/field.go      解析 default tag；DefaultValueInterface 是否为 nil 在此分叉
//	schema/schema.go     HasDefaultValue 且 DefaultValueInterface==nil → 归入 FieldsWithDefaultDBValue
//	callbacks/create.go  ConvertToCreateValues：第一遍确定列清单并完成零值替换回写，
//	                     第二遍仅对 FieldsWithDefaultDBValue 中取值非零的字段追加列
//	clause/set.go        AssignmentColumns 生成 excluded.<col> 引用；Assignments 经 AddVar 绑定参数
//	mysql.go             excluded.<col> → VALUES(<col>) 的方言重写；位于驱动仓 gorm.io/driver/mysql
func (d *GormValidateLogDAO) BatchInsert(ctx context.Context, logs []ValidateLog) error {
	if len(logs) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "task_id"}, {Name: "table_name"}, {Name: "biz_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"mismatch_kind": gorm.Expr("VALUES(`mismatch_kind`)"),
			"diff_detail":   gorm.Expr("VALUES(`diff_detail`)"),
			"created_at":    gorm.Expr("VALUES(`created_at`)"),
			"repaired":      0, // 强制重置；字面值绑定，不依赖 VALUES(repaired)——原理见函数注释
		}),
	}).CreateInBatches(logs, 100).Error
}

func (d *GormValidateLogDAO) FindByIDs(ctx context.Context, ids []int64) ([]ValidateLog, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []ValidateLog
	err := d.db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error
	return list, err
}

func (d *GormValidateLogDAO) ListUnrepaired(ctx context.Context, taskId int64, offset, limit int) ([]ValidateLog, int64, error) {
	db := d.db.WithContext(ctx).Model(&ValidateLog{}).
		Where("task_id = ? AND repaired = 0", taskId)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []ValidateLog
	if err := db.Order("created_at ASC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (d *GormValidateLogDAO) MarkRepaired(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Model(&ValidateLog{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"repaired":    1,
			"repaired_at": time.Now().UnixMilli(),
		}).Error
}

// ── DAO Model ───────────────────────────────────────────

type ValidateLog struct {
	Id           int64  `gorm:"primaryKey;autoIncrement"`
	TaskId       int64  `gorm:"not null;index:idx_validate_log_task_repaired,priority:1;uniqueIndex:uk_validate_log_dedup,priority:1;column:task_id"`
	Direction    string `gorm:"type:varchar(16);not null"`
	BizTable     string `gorm:"type:varchar(64);not null;column:table_name;uniqueIndex:uk_validate_log_dedup,priority:2"`
	BizId        string `gorm:"type:varchar(64);not null;column:biz_id;uniqueIndex:uk_validate_log_dedup,priority:3"`
	MismatchKind string `gorm:"type:varchar(32);not null;column:mismatch_kind"`
	DiffDetail   string `gorm:"type:text;column:diff_detail"`
	Repaired     int8   `gorm:"not null;default:0;index:idx_validate_log_task_repaired,priority:2"`
	CreatedAt    int64  `gorm:"autoCreateTime:milli;column:created_at;index:idx_validate_log_created"`
	RepairedAt   int64  `gorm:"not null;default:0;column:repaired_at"`
}

func (ValidateLog) TableName() string {
	return "validate_log"
}
