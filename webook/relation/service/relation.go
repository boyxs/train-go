package service

import (
	"context"

	"github.com/webook/relation/domain"
	"github.com/webook/relation/errs"
	"github.com/webook/relation/repository"
)

type RelationService interface {
	Follow(ctx context.Context, followerId, followeeId int64) (bool, error)
	Unfollow(ctx context.Context, followerId, followeeId int64) (bool, error)
	Block(ctx context.Context, uid, blockedId int64) (bool, error)
	Unblock(ctx context.Context, uid, blockedId int64) (bool, error)
	GetStats(ctx context.Context, uid int64) (domain.RelationStats, error)
	BatchGetStats(ctx context.Context, uids []int64) (map[int64]domain.RelationStats, error)
	// GetRelation 组装 viewer 对一批 target 的关系态（关注按钮 3 态 / 列表互关标记）。
	GetRelation(ctx context.Context, viewerId int64, targetIds []int64) (map[int64]domain.RelationState, error)
	// 列表方法内部归一 limit（默认 20 / 上限 50）并返回下一页游标（本页最后一条 id，0=无更多）——
	// 分页规则/契约归 service，接入层（gRPC server）只做 pb 映射。
	ListFollowees(ctx context.Context, followerId, cursor int64, limit int) ([]domain.FollowEdge, int64, error)
	ListFollowers(ctx context.Context, followeeId, cursor int64, limit int) ([]domain.FollowEdge, int64, error)
	ListBlocks(ctx context.Context, uid, cursor int64, limit int) ([]domain.BlockEdge, int64, error)
}

const (
	defaultListLimit = 20 // 未传/非法 limit 的默认页大小
	maxListLimit     = 50 // 分页上限（对齐 PRD）
)

type InternalRelationService struct {
	repo repository.RelationRepository
}

func NewInternalRelationService(repo repository.RelationRepository) RelationService {
	return &InternalRelationService{repo: repo}
}

func (s *InternalRelationService) Follow(ctx context.Context, followerId, followeeId int64) (bool, error) {
	if followerId == followeeId {
		return false, errs.ErrFollowSelf
	}
	// 双向拉黑门控（check-then-act：P0 接受关注与拉黑并发的微小竞态，拉黑级联会兜底解除关注）
	blocked, err := s.repo.FindBlockedIn(ctx, followerId, []int64{followeeId})
	if err != nil {
		return false, err
	}
	if len(blocked) > 0 {
		return false, errs.ErrBlockedTarget
	}
	blockedBy, err := s.repo.FindBlockedByIn(ctx, followerId, []int64{followeeId})
	if err != nil {
		return false, err
	}
	if len(blockedBy) > 0 {
		return false, errs.ErrBlockedByTarget
	}
	return s.repo.Follow(ctx, followerId, followeeId)
}

func (s *InternalRelationService) Unfollow(ctx context.Context, followerId, followeeId int64) (bool, error) {
	return s.repo.Unfollow(ctx, followerId, followeeId)
}

func (s *InternalRelationService) Block(ctx context.Context, uid, blockedId int64) (bool, error) {
	if uid == blockedId {
		return false, errs.ErrBlockSelf
	}
	return s.repo.Block(ctx, uid, blockedId)
}

func (s *InternalRelationService) Unblock(ctx context.Context, uid, blockedId int64) (bool, error) {
	return s.repo.Unblock(ctx, uid, blockedId)
}

func (s *InternalRelationService) GetStats(ctx context.Context, uid int64) (domain.RelationStats, error) {
	return s.repo.GetStats(ctx, uid)
}

func (s *InternalRelationService) BatchGetStats(ctx context.Context, uids []int64) (map[int64]domain.RelationStats, error) {
	return s.repo.BatchGetStats(ctx, uids)
}

func (s *InternalRelationService) GetRelation(ctx context.Context, viewerId int64, targetIds []int64) (map[int64]domain.RelationState, error) {
	res := make(map[int64]domain.RelationState, len(targetIds))
	if len(targetIds) == 0 {
		return res, nil
	}
	following, err := s.repo.FindFolloweesIn(ctx, viewerId, targetIds) // viewer 关注的
	if err != nil {
		return nil, err
	}
	followedBy, err := s.repo.FindFollowersIn(ctx, viewerId, targetIds) // 关注 viewer 的
	if err != nil {
		return nil, err
	}
	blocked, err := s.repo.FindBlockedIn(ctx, viewerId, targetIds) // viewer 拉黑的
	if err != nil {
		return nil, err
	}
	blockedBy, err := s.repo.FindBlockedByIn(ctx, viewerId, targetIds) // 拉黑 viewer 的
	if err != nil {
		return nil, err
	}
	fSet, fbSet, bSet, bbSet := toSet(following), toSet(followedBy), toSet(blocked), toSet(blockedBy)
	for _, tid := range targetIds {
		res[tid] = domain.RelationState{
			IsFollowing:  fSet[tid],
			IsFollowedBy: fbSet[tid],
			IsBlocked:    bSet[tid],
			IsBlockedBy:  bbSet[tid],
		}
	}
	return res, nil
}

func toSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func (s *InternalRelationService) ListFollowees(ctx context.Context, followerId, cursor int64, limit int) ([]domain.FollowEdge, int64, error) {
	edges, err := s.repo.ListFollowees(ctx, followerId, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	return edges, followCursor(edges), nil
}

func (s *InternalRelationService) ListFollowers(ctx context.Context, followeeId, cursor int64, limit int) ([]domain.FollowEdge, int64, error) {
	edges, err := s.repo.ListFollowers(ctx, followeeId, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	return edges, followCursor(edges), nil
}

func (s *InternalRelationService) ListBlocks(ctx context.Context, uid, cursor int64, limit int) ([]domain.BlockEdge, int64, error) {
	edges, err := s.repo.ListBlocks(ctx, uid, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	return edges, blockCursor(edges), nil
}

// followCursor / blockCursor 下一页游标 = 本页最后一条边的 id（0=无更多）。
func followCursor(edges []domain.FollowEdge) int64 {
	if n := len(edges); n > 0 {
		return edges[n-1].Id
	}
	return 0
}

func blockCursor(edges []domain.BlockEdge) int64 {
	if n := len(edges); n > 0 {
		return edges[n-1].Id
	}
	return 0
}
