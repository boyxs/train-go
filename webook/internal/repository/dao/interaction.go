package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InteractionDAO interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	UpsertLike(ctx context.Context, uid int64, biz string, bizId int64, liked bool) error
	UpsertCollect(ctx context.Context, uid int64, biz string, bizId int64, collected bool) error
	FindByBizId(ctx context.Context, biz string, bizId int64) (Interaction, error)
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]Interaction, error)
	FindUserInteraction(ctx context.Context, uid int64, biz string, bizId int64) (UserInteraction, error)
}

type GormInteractionDAO struct {
	db *gorm.DB
}

func NewGormInteractionDAO(db *gorm.DB) InteractionDAO {
	return &GormInteractionDAO{db: db}
}

func (d *GormInteractionDAO) IncrReadCount(ctx context.Context, biz string, bizId int64) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		DoUpdates: clause.Assignments(map[string]any{
			"read_count": gorm.Expr("read_count + 1"),
			"updated_at": time.Now().UnixMilli(),
		}),
	}).Create(&Interaction{
		BizId:     bizId,
		Biz:       biz,
		ReadCount: 1,
	}).Error
}

func (d *GormInteractionDAO) UpsertLike(ctx context.Context, uid int64, biz string, bizId int64, liked bool) error {
	now := time.Now().UnixMilli()
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(map[string]any{
				"liked":      liked,
				"updated_at": now,
			}),
		}).Create(&UserInteraction{
			UserId: uid,
			BizId:  bizId,
			Biz:    biz,
			Liked:  liked,
		}).Error
		if err != nil {
			return err
		}
		delta := int64(1)
		if !liked {
			delta = -1
		}
		return tx.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(map[string]any{
				"like_count": gorm.Expr("GREATEST(0, like_count + ?)", delta),
				"updated_at": now,
			}),
		}).Create(&Interaction{
			BizId:     bizId,
			Biz:       biz,
			LikeCount: max(0, delta),
		}).Error
	})
}

func (d *GormInteractionDAO) UpsertCollect(ctx context.Context, uid int64, biz string, bizId int64, collected bool) error {
	now := time.Now().UnixMilli()
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(map[string]any{
				"collected":  collected,
				"updated_at": now,
			}),
		}).Create(&UserInteraction{
			UserId:    uid,
			BizId:     bizId,
			Biz:       biz,
			Collected: collected,
		}).Error
		if err != nil {
			return err
		}
		delta := int64(1)
		if !collected {
			delta = -1
		}
		return tx.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(map[string]any{
				"collect_count": gorm.Expr("GREATEST(0, collect_count + ?)", delta),
				"updated_at":    now,
			}),
		}).Create(&Interaction{
			BizId:        bizId,
			Biz:          biz,
			CollectCount: max(0, delta),
		}).Error
	})
}

func (d *GormInteractionDAO) FindByBizId(ctx context.Context, biz string, bizId int64) (Interaction, error) {
	var intr Interaction
	err := d.db.WithContext(ctx).Where("biz = ? AND biz_id = ?", biz, bizId).First(&intr).Error
	return intr, err
}

func (d *GormInteractionDAO) FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]Interaction, error) {
	var intrs []Interaction
	err := d.db.WithContext(ctx).Where("biz = ? AND biz_id IN ?", biz, bizIds).Find(&intrs).Error
	return intrs, err
}

func (d *GormInteractionDAO) FindUserInteraction(ctx context.Context, uid int64, biz string, bizId int64) (UserInteraction, error) {
	var ui UserInteraction
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND biz = ? AND biz_id = ?", uid, biz, bizId).
		Order("id DESC").
		First(&ui).Error
	return ui, err
}

// Interaction 互动聚合计数表（通用，biz+biz_id 标识业务对象）
type Interaction struct {
	Id           int64  `gorm:"primaryKey,autoIncrement"`
	Biz          string `gorm:"type:varchar(64);uniqueIndex:uk_biz"`
	BizId        int64  `gorm:"uniqueIndex:uk_biz"`
	ReadCount    int64
	LikeCount    int64
	CollectCount int64
	CreatedAt    int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt    int64 `gorm:"autoUpdateTime:milli"`
}

func (Interaction) TableName() string {
	return "interaction"
}

// UserInteraction 用户操作记录表（通用）
type UserInteraction struct {
	Id        int64  `gorm:"primaryKey,autoIncrement"`
	Biz       string `gorm:"type:varchar(64);uniqueIndex:uk_user_biz"`
	BizId     int64  `gorm:"uniqueIndex:uk_user_biz"`
	UserId    int64  `gorm:"uniqueIndex:uk_user_biz"`
	Liked     bool
	Collected bool
	CreatedAt int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli"`
}

func (UserInteraction) TableName() string {
	return "user_interaction"
}
