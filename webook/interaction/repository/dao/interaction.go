package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InteractionDAO interface {
	IncrReadCount(ctx context.Context, biz string, bizId int64) error
	BatchIncrReadCount(ctx context.Context, items []ReadCountItem) error
	UpsertLike(ctx context.Context, uid int64, biz string, bizId int64, liked bool) error
	UpsertCollect(ctx context.Context, uid int64, biz string, bizId int64, collected bool) error
	FindByBizId(ctx context.Context, biz string, bizId int64) (Interaction, error)
	FindByBizIds(ctx context.Context, biz string, bizIds []int64) ([]Interaction, error)
	FindUserInteraction(ctx context.Context, uid int64, biz string, bizId int64) (UserInteraction, error)
	// FindLikedBizIds 批量查用户在 bizIds 中已点赞的 bizId，按需聚合列表 liked 状态（避免 N+1）
	FindLikedBizIds(ctx context.Context, uid int64, biz string, bizIds []int64) ([]int64, error)
	// ListCollectedBizIds 查询用户收藏的 bizId 列表，按收藏时间降序
	ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error)
	// ListHotBizIds 查询热门 bizId 列表，按 read_count + like_count*3 + collect_count*5 降序
	ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error)
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

// ReadCountItem 批量累加项（dao 层，不依赖 domain）。
type ReadCountItem struct {
	Biz   string
	BizId int64
	Count int64
}

// BatchIncrReadCount 单事务内逐项 upsert 累加 read_count（一批通常 ≤ 消费批大小，N 小）。
// 整批原子提交：消费侧据此「一批一次 RPC、失败整批重投」，杜绝批内部分成功导致的重复计数。
func (d *GormInteractionDAO) BatchIncrReadCount(ctx context.Context, items []ReadCountItem) error {
	if len(items) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			if err := tx.Clauses(clause.OnConflict{
				DoUpdates: clause.Assignments(map[string]any{
					"read_count": gorm.Expr("read_count + ?", it.Count),
					"updated_at": now,
				}),
			}).Create(&Interaction{
				BizId:     it.BizId,
				Biz:       it.Biz,
				ReadCount: it.Count,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *GormInteractionDAO) UpsertLike(ctx context.Context, uid int64, biz string, bizId int64, liked bool) error {
	now := time.Now().UnixMilli()
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先查旧状态判断 liked 是否真翻转：updated_at 每次都给新值，ON DUPLICATE KEY 的 RowsAffected 恒为 2，
		// 不能用它判幂等（否则重复点赞会一直累加）。
		// FOR UPDATE 行锁：并发同用户点赞若不加锁会都读到旧值 false 各 +1 → 计数翻倍；
		// 行不存在时唯一索引上的间隙锁同样串行化两个事务，保证只在真实翻转时改计数。
		var existing UserInteraction
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND biz = ? AND biz_id = ?", uid, biz, bizId).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		oldLiked := err == nil && existing.Liked
		if uErr := tx.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(map[string]any{
				"liked":      liked,
				"updated_at": now,
			}),
		}).Create(&UserInteraction{
			UserId: uid,
			BizId:  bizId,
			Biz:    biz,
			Liked:  liked,
		}).Error; uErr != nil {
			return uErr
		}
		// 状态没翻转（重复点赞/重复取消），幂等跳过计数
		if oldLiked == liked {
			return nil
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
		// 先查旧状态判断 collected 是否真翻转：updated_at 每次都给新值，ON DUPLICATE KEY 的 RowsAffected 恒为 2，
		// 不能用它判幂等（否则重复收藏会一直累加）。
		// FOR UPDATE 行锁：理由同 UpsertLike，串行化并发同用户收藏翻转，避免计数翻倍。
		var existing UserInteraction
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND biz = ? AND biz_id = ?", uid, biz, bizId).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		oldCollected := err == nil && existing.Collected
		if uErr := tx.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(map[string]any{
				"collected":  collected,
				"updated_at": now,
			}),
		}).Create(&UserInteraction{
			UserId:    uid,
			BizId:     bizId,
			Biz:       biz,
			Collected: collected,
		}).Error; uErr != nil {
			return uErr
		}
		// 状态没翻转（重复收藏/重复取消），幂等跳过计数
		if oldCollected == collected {
			return nil
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
	// (biz, biz_id, user_id) 唯一索引保证至多一行，去掉误导性的 Order("id DESC")
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND biz = ? AND biz_id = ?", uid, biz, bizId).
		First(&ui).Error
	return ui, err
}

func (d *GormInteractionDAO) FindLikedBizIds(ctx context.Context, uid int64, biz string, bizIds []int64) ([]int64, error) {
	if len(bizIds) == 0 {
		return nil, nil
	}
	var ids []int64
	err := d.db.WithContext(ctx).
		Model(&UserInteraction{}).
		Select("biz_id").
		Where("user_id = ? AND biz = ? AND liked = ? AND biz_id IN ?", uid, biz, true, bizIds).
		Pluck("biz_id", &ids).Error
	return ids, err
}

func (d *GormInteractionDAO) ListCollectedBizIds(ctx context.Context, uid int64, biz string, limit int) ([]int64, error) {
	var ids []int64
	err := d.db.WithContext(ctx).
		Model(&UserInteraction{}).
		Select("biz_id").
		Where("user_id = ? AND biz = ? AND collected = ?", uid, biz, true).
		Order("updated_at DESC").
		Limit(limit).
		Pluck("biz_id", &ids).Error
	return ids, err
}

func (d *GormInteractionDAO) ListHotBizIds(ctx context.Context, biz string, limit int) ([]int64, error) {
	var ids []int64
	err := d.db.WithContext(ctx).
		Model(&Interaction{}).
		Select("biz_id").
		Where("biz = ?", biz).
		Order("read_count + like_count * 3 + collect_count * 5 DESC").
		Limit(limit).
		Pluck("biz_id", &ids).Error
	return ids, err
}

// Interaction 互动聚合计数表（通用，biz+biz_id 标识业务对象）
type Interaction struct {
	Id           int64  `gorm:"primaryKey,autoIncrement"`
	Biz          string `gorm:"type:varchar(64);uniqueIndex:uk_interaction_biz"`
	BizId        int64  `gorm:"uniqueIndex:uk_interaction_biz"`
	ReadCount    int64
	LikeCount    int64
	CollectCount int64
	CreatedAt    int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt    int64 `gorm:"autoUpdateTime:milli"`
	// interaction 无删除路径，刻意不加 gorm soft_delete：它会给所有 SELECT 注入 deleted_at=0，
	// 把 AutoMigrate 加列后未回填的既有 NULL 行全部过滤掉 → 计数/状态查不出来（详见 integration 回归测试）。
}

func (Interaction) TableName() string {
	return "interaction"
}

// UserInteraction 用户操作记录表（通用）
type UserInteraction struct {
	Id        int64  `gorm:"primaryKey,autoIncrement"`
	Biz       string `gorm:"type:varchar(64);uniqueIndex:uk_user_interaction_biz"`
	BizId     int64  `gorm:"uniqueIndex:uk_user_interaction_biz"`
	UserId    int64  `gorm:"uniqueIndex:uk_user_interaction_biz"`
	Liked     bool
	Collected bool
	CreatedAt int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli"`
	// 同 Interaction：无删除路径，不加 soft_delete（避免 deleted_at=0 过滤排除既有 NULL 行）。
}

func (UserInteraction) TableName() string {
	return "user_interaction"
}
