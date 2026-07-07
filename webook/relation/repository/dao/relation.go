package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RelationDAO 用户关系数据访问：关注边（status 翻转）+ 聚合计数 + 黑名单。
// 写方法返回 changed——是否发生真实状态翻转，供上层门控事件生产（重复关注/拉黑幂等，changed=false 不发事件）。
type RelationDAO interface {
	// Follow 关注：幂等；仅在 0→1 真翻转时 changed=true，并维护双方计数。
	Follow(ctx context.Context, followerId, followeeId int64) (changed bool, err error)
	// Unfollow 取关：幂等；仅在 1→0 真翻转时 changed=true。
	Unfollow(ctx context.Context, followerId, followeeId int64) (changed bool, err error)
	// Block 拉黑：幂等；首次拉黑 changed=true，并级联解除双向关注（连带计数）。
	Block(ctx context.Context, uid, blockedId int64) (changed bool, err error)
	// Unblock 取消拉黑：幂等；存在才删，删得掉 changed=true；不恢复关注。
	Unblock(ctx context.Context, uid, blockedId int64) (changed bool, err error)

	// GetStats 读某用户的关注数/粉丝数；无记录返回零值。
	GetStats(ctx context.Context, uid int64) (RelationStats, error)
	// BatchGetStats 批量读计数（列表页聚合用）；无记录的 uid 不在结果里。
	BatchGetStats(ctx context.Context, uids []int64) ([]RelationStats, error)
	// ListFollowees 我关注的（status=1，id 游标倒序；cursor<=0 取首页）。
	ListFollowees(ctx context.Context, followerId, cursor int64, limit int) ([]FollowRelation, error)
	// ListFollowers 我的粉丝（status=1，id 游标倒序）。
	ListFollowers(ctx context.Context, followeeId, cursor int64, limit int) ([]FollowRelation, error)
	// ListBlocks 我的黑名单（id 游标倒序）。
	ListBlocks(ctx context.Context, uid, cursor int64, limit int) ([]BlockRelation, error)
	// FindFolloweesIn 在 followeeIds 中筛出 followerId 正在关注的（组关系态用）。
	FindFolloweesIn(ctx context.Context, followerId int64, followeeIds []int64) ([]int64, error)
	// FindFollowersIn 在 followerIds 中筛出正在关注 followeeId 的。
	FindFollowersIn(ctx context.Context, followeeId int64, followerIds []int64) ([]int64, error)
	// FindBlockedIn 在 targetIds 中筛出 uid 已拉黑的。
	FindBlockedIn(ctx context.Context, uid int64, targetIds []int64) ([]int64, error)
	// FindBlockedByIn 在 blockerIds 中筛出拉黑了 uid 的。
	FindBlockedByIn(ctx context.Context, uid int64, blockerIds []int64) ([]int64, error)
}

type GormRelationDAO struct {
	db *gorm.DB
}

func NewGormRelationDAO(db *gorm.DB) RelationDAO {
	return &GormRelationDAO{db: db}
}

func (d *GormRelationDAO) Follow(ctx context.Context, followerId, followeeId int64) (bool, error) {
	return d.setFollow(ctx, followerId, followeeId, true)
}

func (d *GormRelationDAO) Unfollow(ctx context.Context, followerId, followeeId int64) (bool, error) {
	return d.setFollow(ctx, followerId, followeeId, false)
}

// setFollow 关注(active=true)/取关(active=false) 统一入口：包一层事务后委托 flipFollowTx。
func (d *GormRelationDAO) setFollow(ctx context.Context, followerId, followeeId int64, active bool) (bool, error) {
	var changed bool
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, e := d.flipFollowTx(tx, followerId, followeeId, active)
		changed = c
		return e
	})
	return changed, err
}

// flipFollowTx 在给定事务内翻转一条关注边并维护双方计数，返回是否真翻转。
// FOR UPDATE 行锁串行化并发同一对操作，避免都读旧值各自增减导致计数翻倍/漏减；
// 仅在 oldActive != active 时 upsert 边 + 增减计数（重复关注/取关未关注幂等，不建行不动计数）。
// 供 setFollow（单边）与 Block（级联双向取关）复用。
func (d *GormRelationDAO) flipFollowTx(tx *gorm.DB, followerId, followeeId int64, active bool) (bool, error) {
	var existing FollowRelation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("follower_id = ? AND followee_id = ?", followerId, followeeId).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	oldActive := err == nil && existing.Status == 1
	if oldActive == active {
		return false, nil
	}
	newStatus := uint8(0)
	if active {
		newStatus = 1
	}
	now := time.Now().UnixMilli()
	if uErr := tx.Clauses(clause.OnConflict{
		DoUpdates: clause.Assignments(map[string]any{"status": newStatus, "updated_at": now}),
	}).Create(&FollowRelation{FollowerId: followerId, FolloweeId: followeeId, Status: newStatus}).Error; uErr != nil {
		return false, uErr
	}
	delta, seed := int64(1), int64(1)
	if !active {
		delta, seed = -1, 0
	}
	// 被关注者粉丝数
	if e := tx.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{
		"follower_cnt": gorm.Expr("GREATEST(0, follower_cnt + ?)", delta), "updated_at": now,
	})}).Create(&RelationStats{Uid: followeeId, FollowerCnt: seed}).Error; e != nil {
		return false, e
	}
	// 关注者关注数
	if e := tx.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{
		"followee_cnt": gorm.Expr("GREATEST(0, followee_cnt + ?)", delta), "updated_at": now,
	})}).Create(&RelationStats{Uid: followerId, FolloweeCnt: seed}).Error; e != nil {
		return false, e
	}
	return true, nil
}

// Block 拉黑：幂等插黑名单，首次拉黑 changed=true 并级联解除双向关注（同事务，复用 flipFollowTx）。
func (d *GormRelationDAO) Block(ctx context.Context, uid, blockedId int64) (bool, error) {
	var changed bool
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// DoNothing → INSERT IGNORE：已存在则 RowsAffected=0，视为幂等不再级联。
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&BlockRelation{Uid: uid, BlockedUid: blockedId})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		changed = true
		if _, e := d.flipFollowTx(tx, uid, blockedId, false); e != nil {
			return e
		}
		_, e := d.flipFollowTx(tx, blockedId, uid, false)
		return e
	})
	return changed, err
}

// Unblock 取消拉黑：删黑名单行；删得掉 changed=true。不恢复先前的关注关系。
func (d *GormRelationDAO) Unblock(ctx context.Context, uid, blockedId int64) (bool, error) {
	res := d.db.WithContext(ctx).
		Where("uid = ? AND blocked_uid = ?", uid, blockedId).
		Delete(&BlockRelation{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (d *GormRelationDAO) GetStats(ctx context.Context, uid int64) (RelationStats, error) {
	var st RelationStats
	err := d.db.WithContext(ctx).Where("uid = ?", uid).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RelationStats{}, nil // 无记录返回零值，不当错误（计数天然为 0）
	}
	return st, err
}

func (d *GormRelationDAO) BatchGetStats(ctx context.Context, uids []int64) ([]RelationStats, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	var list []RelationStats
	err := d.db.WithContext(ctx).Where("uid IN ?", uids).Find(&list).Error
	return list, err
}

func (d *GormRelationDAO) ListFollowees(ctx context.Context, followerId, cursor int64, limit int) ([]FollowRelation, error) {
	return d.listEdges(ctx, "follower_id = ?", followerId, cursor, limit)
}

func (d *GormRelationDAO) ListFollowers(ctx context.Context, followeeId, cursor int64, limit int) ([]FollowRelation, error) {
	return d.listEdges(ctx, "followee_id = ?", followeeId, cursor, limit)
}

// listEdges 关注边游标分页（status=1，id DESC；cursor>0 时取 id < cursor）。
func (d *GormRelationDAO) listEdges(ctx context.Context, cond string, id, cursor int64, limit int) ([]FollowRelation, error) {
	q := d.db.WithContext(ctx).Where(cond, id).Where("status = ?", 1)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var list []FollowRelation
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (d *GormRelationDAO) ListBlocks(ctx context.Context, uid, cursor int64, limit int) ([]BlockRelation, error) {
	q := d.db.WithContext(ctx).Where("uid = ?", uid)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var list []BlockRelation
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (d *GormRelationDAO) FindFolloweesIn(ctx context.Context, followerId int64, followeeIds []int64) ([]int64, error) {
	if len(followeeIds) == 0 {
		return nil, nil
	}
	var ids []int64
	err := d.db.WithContext(ctx).Model(&FollowRelation{}).
		Where("follower_id = ? AND status = 1 AND followee_id IN ?", followerId, followeeIds).
		Pluck("followee_id", &ids).Error
	return ids, err
}

func (d *GormRelationDAO) FindFollowersIn(ctx context.Context, followeeId int64, followerIds []int64) ([]int64, error) {
	if len(followerIds) == 0 {
		return nil, nil
	}
	var ids []int64
	err := d.db.WithContext(ctx).Model(&FollowRelation{}).
		Where("followee_id = ? AND status = 1 AND follower_id IN ?", followeeId, followerIds).
		Pluck("follower_id", &ids).Error
	return ids, err
}

func (d *GormRelationDAO) FindBlockedIn(ctx context.Context, uid int64, targetIds []int64) ([]int64, error) {
	if len(targetIds) == 0 {
		return nil, nil
	}
	var ids []int64
	err := d.db.WithContext(ctx).Model(&BlockRelation{}).
		Where("uid = ? AND blocked_uid IN ?", uid, targetIds).
		Pluck("blocked_uid", &ids).Error
	return ids, err
}

func (d *GormRelationDAO) FindBlockedByIn(ctx context.Context, uid int64, blockerIds []int64) ([]int64, error) {
	if len(blockerIds) == 0 {
		return nil, nil
	}
	var ids []int64
	err := d.db.WithContext(ctx).Model(&BlockRelation{}).
		Where("blocked_uid = ? AND uid IN ?", uid, blockerIds).
		Pluck("uid", &ids).Error
	return ids, err
}

// FollowRelation 关注边（单向 follower → followee）。status 翻转维护，不物理删。
type FollowRelation struct {
	Id         int64 `gorm:"primaryKey;autoIncrement"`
	FollowerId int64 `gorm:"uniqueIndex:uk_relation_follow_edge,priority:1;index:idx_relation_follow_er,priority:1"`
	FolloweeId int64 `gorm:"uniqueIndex:uk_relation_follow_edge,priority:2;index:idx_relation_follow_ee,priority:1"`
	Status     uint8 `gorm:"index:idx_relation_follow_er,priority:2;index:idx_relation_follow_ee,priority:2"` // 1=关注中 0=已取关
	CreatedAt  int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt  int64 `gorm:"autoUpdateTime:milli"`
}

func (FollowRelation) TableName() string { return "relation_follow" }

// RelationStats 用户关系聚合计数（每用户一行；无删除路径，不加 soft_delete，同 interaction 计数表）。
type RelationStats struct {
	Id          int64 `gorm:"primaryKey;autoIncrement"`
	Uid         int64 `gorm:"uniqueIndex:uk_relation_stats_uid"`
	FolloweeCnt int64 // 关注数：该用户关注了多少人
	FollowerCnt int64 // 粉丝数：多少人关注该用户
	CreatedAt   int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt   int64 `gorm:"autoUpdateTime:milli"`
}

func (RelationStats) TableName() string { return "relation_stats" }

// BlockRelation 黑名单（uid 拉黑 blocked_uid；低频，取消=物理删）。
type BlockRelation struct {
	Id         int64 `gorm:"primaryKey;autoIncrement"`
	Uid        int64 `gorm:"uniqueIndex:uk_relation_block_edge,priority:1;index:idx_relation_block_uid"`
	BlockedUid int64 `gorm:"uniqueIndex:uk_relation_block_edge,priority:2"`
	CreatedAt  int64 `gorm:"autoCreateTime:milli"`
}

func (BlockRelation) TableName() string { return "relation_block" }
