package repository

import (
	"context"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
	"github.com/boyxs/train-go/webook/relation/domain"
	"github.com/boyxs/train-go/webook/relation/repository/cache"
	"github.com/boyxs/train-go/webook/relation/repository/dao"
)

type RelationRepository interface {
	Follow(ctx context.Context, followerId, followeeId int64) (bool, error)
	Unfollow(ctx context.Context, followerId, followeeId int64) (bool, error)
	Block(ctx context.Context, uid, blockedId int64) (bool, error)
	Unblock(ctx context.Context, uid, blockedId int64) (bool, error)
	GetStats(ctx context.Context, uid int64) (domain.RelationStats, error)
	BatchGetStats(ctx context.Context, uids []int64) (map[int64]domain.RelationStats, error)
	ListFollowees(ctx context.Context, followerId, cursor int64, limit int) ([]domain.FollowEdge, error)
	ListFollowers(ctx context.Context, followeeId, cursor int64, limit int) ([]domain.FollowEdge, error)
	ListBlocks(ctx context.Context, uid, cursor int64, limit int) ([]domain.BlockEdge, error)
	FindFolloweesIn(ctx context.Context, followerId int64, followeeIds []int64) ([]int64, error)
	FindFollowersIn(ctx context.Context, followeeId int64, followerIds []int64) ([]int64, error)
	FindBlockedIn(ctx context.Context, uid int64, targetIds []int64) ([]int64, error)
	FindBlockedByIn(ctx context.Context, uid int64, blockerIds []int64) ([]int64, error)
}

type CacheRelationRepository struct {
	dao   dao.RelationDAO
	cache cache.RelationCache
	l     logger.LoggerX
}

func NewCacheRelationRepository(d dao.RelationDAO, c cache.RelationCache, l logger.LoggerX) RelationRepository {
	return &CacheRelationRepository{dao: d, cache: c, l: l}
}

func (r *CacheRelationRepository) Follow(ctx context.Context, followerId, followeeId int64) (bool, error) {
	changed, err := r.dao.Follow(ctx, followerId, followeeId)
	if err == nil && changed {
		r.delStats(ctx, followerId, followeeId) // 关注变更改了双方计数，清双方缓存
	}
	return changed, err
}

func (r *CacheRelationRepository) Unfollow(ctx context.Context, followerId, followeeId int64) (bool, error) {
	changed, err := r.dao.Unfollow(ctx, followerId, followeeId)
	if err == nil && changed {
		r.delStats(ctx, followerId, followeeId)
	}
	return changed, err
}

func (r *CacheRelationRepository) Block(ctx context.Context, uid, blockedId int64) (bool, error) {
	changed, err := r.dao.Block(ctx, uid, blockedId)
	if err == nil && changed {
		r.delStats(ctx, uid, blockedId) // 级联解除双向关注，清双方缓存
	}
	return changed, err
}

func (r *CacheRelationRepository) Unblock(ctx context.Context, uid, blockedId int64) (bool, error) {
	return r.dao.Unblock(ctx, uid, blockedId) // 取消拉黑不改计数，无需清缓存
}

func (r *CacheRelationRepository) GetStats(ctx context.Context, uid int64) (domain.RelationStats, error) {
	if st, err := r.cache.GetStats(ctx, uid); err == nil {
		return st, nil
	}
	// 未命中（redis.Nil）或缓存故障都回源 DB
	daoSt, err := r.dao.GetStats(ctx, uid)
	if err != nil {
		return domain.RelationStats{}, err
	}
	res := r.toDomainStats(daoSt)
	res.Uid = uid
	if setErr := r.cache.SetStats(ctx, res); setErr != nil {
		r.l.Error("回填关系计数缓存失败", logger.Int64("uid", uid), logger.Error(setErr))
	}
	return res, nil
}

func (r *CacheRelationRepository) delStats(ctx context.Context, uids ...int64) {
	if err := r.cache.DelStats(ctx, uids...); err != nil {
		r.l.Error("删除关系计数缓存失败", logger.Error(err))
	}
}

func (r *CacheRelationRepository) BatchGetStats(ctx context.Context, uids []int64) (map[int64]domain.RelationStats, error) {
	list, err := r.dao.BatchGetStats(ctx, uids)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]domain.RelationStats, len(list))
	for _, e := range list {
		m[e.Uid] = r.toDomainStats(e)
	}
	return m, nil
}

func (r *CacheRelationRepository) ListFollowees(ctx context.Context, followerId, cursor int64, limit int) ([]domain.FollowEdge, error) {
	list, err := r.dao.ListFollowees(ctx, followerId, cursor, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomainEdge), nil
}

func (r *CacheRelationRepository) ListFollowers(ctx context.Context, followeeId, cursor int64, limit int) ([]domain.FollowEdge, error) {
	list, err := r.dao.ListFollowers(ctx, followeeId, cursor, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomainEdge), nil
}

func (r *CacheRelationRepository) ListBlocks(ctx context.Context, uid, cursor int64, limit int) ([]domain.BlockEdge, error) {
	list, err := r.dao.ListBlocks(ctx, uid, cursor, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomainBlock), nil
}

func (r *CacheRelationRepository) FindFolloweesIn(ctx context.Context, followerId int64, followeeIds []int64) ([]int64, error) {
	return r.dao.FindFolloweesIn(ctx, followerId, followeeIds)
}

func (r *CacheRelationRepository) FindFollowersIn(ctx context.Context, followeeId int64, followerIds []int64) ([]int64, error) {
	return r.dao.FindFollowersIn(ctx, followeeId, followerIds)
}

func (r *CacheRelationRepository) FindBlockedIn(ctx context.Context, uid int64, targetIds []int64) ([]int64, error) {
	return r.dao.FindBlockedIn(ctx, uid, targetIds)
}

func (r *CacheRelationRepository) FindBlockedByIn(ctx context.Context, uid int64, blockerIds []int64) ([]int64, error) {
	return r.dao.FindBlockedByIn(ctx, uid, blockerIds)
}

// ── dao → domain 单条转换（方向前缀 toDomain + 类型后缀；批量走 slicex.Map）──────────────
func (r *CacheRelationRepository) toDomainStats(e dao.RelationStats) domain.RelationStats {
	return domain.RelationStats{Uid: e.Uid, FolloweeCnt: e.FolloweeCnt, FollowerCnt: e.FollowerCnt}
}

func (r *CacheRelationRepository) toDomainEdge(e dao.FollowRelation) domain.FollowEdge {
	return domain.FollowEdge{Id: e.Id, FollowerId: e.FollowerId, FolloweeId: e.FolloweeId, CreatedAt: e.CreatedAt}
}

func (r *CacheRelationRepository) toDomainBlock(e dao.BlockRelation) domain.BlockEdge {
	return domain.BlockEdge{Id: e.Id, Uid: e.Uid, BlockedUid: e.BlockedUid, CreatedAt: e.CreatedAt}
}
