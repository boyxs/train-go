package dao

import (
	"context"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"

	"github.com/webook/migrator/errs"
)

type TaskDAO interface {
	Insert(ctx context.Context, t Task) (int64, error)
	FindById(ctx context.Context, id int64) (Task, error)
	FindByName(ctx context.Context, name string) (Task, error)
	List(ctx context.Context, status *int8, offset, limit int) ([]Task, int64, error)
	UpdateStatus(ctx context.Context, id int64, status int8) error
	UpdateGrayPercent(ctx context.Context, id int64, percent int16) error
	SoftDelete(ctx context.Context, id int64) error
}

type GormTaskDAO struct {
	db *gorm.DB
}

func NewGormTaskDAO(db *gorm.DB) TaskDAO {
	return &GormTaskDAO{db: db}
}

func (d *GormTaskDAO) Insert(ctx context.Context, t Task) (int64, error) {
	err := d.db.WithContext(ctx).Create(&t).Error
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		const uniqueErrNo uint16 = 1062
		if mysqlErr.Number == uniqueErrNo && strings.Contains(mysqlErr.Message, "uni_task_name") {
			return 0, errs.ErrDuplicateTaskName
		}
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	return t.Id, nil
}

func (d *GormTaskDAO) FindById(ctx context.Context, id int64) (Task, error) {
	var t Task
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Task{}, errs.ErrTaskNotFound
	}
	return t, err
}

func (d *GormTaskDAO) FindByName(ctx context.Context, name string) (Task, error) {
	var t Task
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Task{}, errs.ErrTaskNotFound
	}
	return t, err
}

func (d *GormTaskDAO) List(ctx context.Context, status *int8, offset, limit int) ([]Task, int64, error) {
	db := d.db.WithContext(ctx).Model(&Task{})
	if status != nil {
		db = db.Where("status = ?", *status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Task
	if err := db.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (d *GormTaskDAO) UpdateStatus(ctx context.Context, id int64, status int8) error {
	return d.db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).Update("status", status).Error
}

func (d *GormTaskDAO) UpdateGrayPercent(ctx context.Context, id int64, percent int16) error {
	return d.db.WithContext(ctx).Model(&Task{}).
		Where("id = ?", id).Update("gray_percent", percent).Error
}

func (d *GormTaskDAO) SoftDelete(ctx context.Context, id int64) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&Task{}).Error
}

// ── DAO Models ───────────────────────────────────────────

type Task struct {
	Id           int64                 `gorm:"primaryKey;autoIncrement"`
	Name         string                `gorm:"type:varchar(128);not null;uniqueIndex:uni_task_name"`
	Mode         string                `gorm:"type:varchar(16);not null"`
	Kind         string                `gorm:"type:varchar(32);not null"`
	SourceType   string                `gorm:"type:varchar(32);not null;default:'mysql';column:source_type"`
	SourceDsnRef string                `gorm:"type:varchar(64);not null;column:source_dsn_ref"`
	SinkType     string                `gorm:"type:varchar(32);not null;column:sink_type"`
	SinkDsnRef   string                `gorm:"type:varchar(64);not null;column:sink_dsn_ref"`
	TablesJSON   string                `gorm:"type:text;not null;column:tables_json"`
	Status       int8                  `gorm:"not null;default:0;index:idx_task_status"`
	GrayPercent  int16                 `gorm:"not null;default:0;column:gray_percent"`
	Consistency  string                `gorm:"type:varchar(16);not null;default:'eventual'"`
	CreatedAt    int64                 `gorm:"autoCreateTime:milli;column:created_at"`
	UpdatedAt    int64                 `gorm:"autoUpdateTime:milli;column:updated_at"`
	DeletedAt    soft_delete.DeletedAt `gorm:"softDelete:milli;column:deleted_at"`
}

func (Task) TableName() string {
	return "task"
}
