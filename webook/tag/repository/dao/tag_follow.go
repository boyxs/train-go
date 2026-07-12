package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TagFollowDAO 用户关注标签边（uid → tag_id）：status 翻转（不物理删）+ 同步 tag.follow_count。
// Follow/Unfollow 幂等——仅真翻转（0→1 / 1→0）时增减计数（GREATEST 防负），返回翻转后的关注数。
type TagFollowDAO interface {
	// Follow 关注：幂等；仅 0→1 真翻转时 changed=true 并 follow_count+1；返回当前关注数。
	Follow(ctx context.Context, uid, tagId int64) (changed bool, followerCount int64, err error)
	// Unfollow 取关：幂等；仅 1→0 真翻转时 changed=true 并 follow_count-1；返回当前关注数。
	Unfollow(ctx context.Context, uid, tagId int64) (changed bool, followerCount int64, err error)
	// IsFollowing viewer 是否正在关注该标签（status=1 点查）。
	IsFollowing(ctx context.Context, uid, tagId int64) (bool, error)
}

type GormTagFollowDAO struct {
	db *gorm.DB
}

func NewGormTagFollowDAO(db *gorm.DB) TagFollowDAO {
	return &GormTagFollowDAO{db: db}
}

func (d *GormTagFollowDAO) Follow(ctx context.Context, uid, tagId int64) (bool, int64, error) {
	return d.setFollow(ctx, uid, tagId, true)
}

func (d *GormTagFollowDAO) Unfollow(ctx context.Context, uid, tagId int64) (bool, int64, error) {
	return d.setFollow(ctx, uid, tagId, false)
}

// setFollow 事务内翻转 (uid,tagId) 关注边并维护 tag.follow_count，返回是否真翻转 + 翻转后关注数。
// FOR UPDATE 行锁串行化并发同一对操作，避免各读旧值增减导致计数翻倍/漏减；
// 仅 oldActive != active 时 upsert 边 + 增减计数（重复关注/取关未关注幂等，不动计数）。
func (d *GormTagFollowDAO) setFollow(ctx context.Context, uid, tagId int64, active bool) (bool, int64, error) {
	var (
		changed bool
		count   int64
	)
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing TagFollow
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uid = ? AND tag_id = ?", uid, tagId).First(&existing).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		oldActive := e == nil && existing.Status == 1
		now := time.Now().UnixMilli()
		if oldActive != active {
			newStatus := uint8(0)
			if active {
				newStatus = 1
			}
			if uErr := tx.Clauses(clause.OnConflict{
				DoUpdates: clause.Assignments(map[string]any{"status": newStatus, "updated_at": now}),
			}).Create(&TagFollow{Uid: uid, TagId: tagId, Status: newStatus}).Error; uErr != nil {
				return uErr
			}
			delta := int64(1)
			if !active {
				delta = -1
			}
			if uErr := tx.Model(&Tag{}).Where("id = ?", tagId).
				Updates(map[string]any{
					"follow_count": gorm.Expr("GREATEST(0, follow_count + ?)", delta),
					"updated_at":   now,
				}).Error; uErr != nil {
				return uErr
			}
			changed = true
		}
		// 回读当前关注数（真翻转 / 幂等都返当前值，供上层直接回显）
		return tx.Model(&Tag{}).Where("id = ?", tagId).Pluck("follow_count", &count).Error
	})
	return changed, count, err
}

func (d *GormTagFollowDAO) IsFollowing(ctx context.Context, uid, tagId int64) (bool, error) {
	var cnt int64
	err := d.db.WithContext(ctx).Model(&TagFollow{}).
		Where("uid = ? AND tag_id = ? AND status = 1", uid, tagId).
		Count(&cnt).Error
	return cnt > 0, err
}

// TagFollow 用户关注标签边（uid → tag_id）。status 翻转维护，不物理删。
type TagFollow struct {
	Id        int64 `gorm:"primaryKey;autoIncrement"`
	Uid       int64 `gorm:"not null;default:0;uniqueIndex:uk_tag_follow_edge,priority:1;index:idx_tag_follow_uid,priority:1"`
	TagId     int64 `gorm:"not null;default:0;uniqueIndex:uk_tag_follow_edge,priority:2"`
	Status    uint8 `gorm:"not null;default:0;index:idx_tag_follow_uid,priority:2"` // 1=关注中 0=已取关
	CreatedAt int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli"`
}

func (TagFollow) TableName() string { return "tag_follow" }
