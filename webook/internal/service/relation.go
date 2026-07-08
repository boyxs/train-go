package service

import (
	"context"
	"time"

	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	relationevt "github.com/boyxs/train-go/webook/internal/events/relation"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// RelationService 用户关系 core 网关业务：调 relation gRPC 后端，聚合 user 昵称/简介 + 关系态 + 每人计数，
// 写成功且真变更（changed）时经 core 生产 relation_events（relation 服务纯同步不产事件）。
// 接入层（web handler）只负责 req 绑定 + 调本 service + domain→VO 映射。
type RelationService interface {
	Follow(ctx context.Context, followerId, followeeId int64) error
	Unfollow(ctx context.Context, followerId, followeeId int64) error
	Block(ctx context.Context, uid, targetId int64) error
	Unblock(ctx context.Context, uid, targetId int64) error
	// Followees 列表主人关注的人（含每人计数 + 是否互关）
	Followees(ctx context.Context, userId, cursor int64, limit int32) ([]domain.RelationUser, int64, error)
	// Followers 列表主人的粉丝（含每人计数 + 是否已回关 + 关注时间）
	Followers(ctx context.Context, userId, cursor int64, limit int32) ([]domain.RelationUser, int64, error)
	// Blocklist uid 的黑名单（含拉黑时间）
	Blocklist(ctx context.Context, uid, cursor int64, limit int32) ([]domain.RelationUser, int64, error)
	// Stat 某用户计数 + viewer 关系态（viewer<=0 或 == userId 时只回计数）
	Stat(ctx context.Context, viewerId, userId int64) (domain.RelationStat, error)
}

type GRPCRelationService struct {
	client   relationv1.RelationServiceClient
	userSvc  UserService
	producer relationevt.RelationEventProducer
	l        logger.LoggerX
}

func NewGRPCRelationService(
	client relationv1.RelationServiceClient,
	userSvc UserService,
	producer relationevt.RelationEventProducer,
	l logger.LoggerX,
) RelationService {
	return &GRPCRelationService{client: client, userSvc: userSvc, producer: producer, l: l}
}

func (s *GRPCRelationService) Follow(ctx context.Context, followerId, followeeId int64) error {
	resp, err := s.client.Follow(ctx, &relationv1.FollowRequest{FollowerId: followerId, FolloweeId: followeeId})
	if err != nil {
		return err
	}
	if resp.GetChanged() {
		s.produce(ctx, relationevt.TypeFollow, followerId, followeeId)
	}
	return nil
}

func (s *GRPCRelationService) Unfollow(ctx context.Context, followerId, followeeId int64) error {
	resp, err := s.client.Unfollow(ctx, &relationv1.FollowRequest{FollowerId: followerId, FolloweeId: followeeId})
	if err != nil {
		return err
	}
	if resp.GetChanged() {
		s.produce(ctx, relationevt.TypeUnfollow, followerId, followeeId)
	}
	return nil
}

func (s *GRPCRelationService) Block(ctx context.Context, uid, targetId int64) error {
	resp, err := s.client.Block(ctx, &relationv1.BlockRequest{Uid: uid, BlockedId: targetId})
	if err != nil {
		return err
	}
	if resp.GetChanged() {
		s.produce(ctx, relationevt.TypeBlock, uid, targetId)
	}
	return nil
}

func (s *GRPCRelationService) Unblock(ctx context.Context, uid, targetId int64) error {
	resp, err := s.client.Unblock(ctx, &relationv1.BlockRequest{Uid: uid, BlockedId: targetId})
	if err != nil {
		return err
	}
	if resp.GetChanged() {
		s.produce(ctx, relationevt.TypeUnblock, uid, targetId)
	}
	return nil
}

func (s *GRPCRelationService) Followees(ctx context.Context, userId, cursor int64, limit int32) ([]domain.RelationUser, int64, error) {
	resp, err := s.client.ListFollowees(ctx, &relationv1.ListRequest{Uid: userId, Cursor: cursor, Limit: limit})
	if err != nil {
		return nil, 0, err
	}
	edges := resp.GetEdges()
	ids := make([]int64, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, e.GetFolloweeId())
	}
	users := s.resolveUsers(ctx, ids)
	states := s.relationStates(ctx, userId, ids) // viewer=列表主人：判互关
	stats := s.batchStats(ctx, ids)
	items := make([]domain.RelationUser, 0, len(edges))
	for _, e := range edges {
		tid := e.GetFolloweeId()
		u := users[tid]
		st := states[tid]
		cnt := stats[tid]
		items = append(items, domain.RelationUser{
			Id:          tid,
			Name:        u.Nickname,
			Bio:         u.AboutMe,
			IsMutual:    st.GetIsFollowing() && st.GetIsFollowedBy(),
			FolloweeCnt: cnt.GetFolloweeCnt(),
			FollowerCnt: cnt.GetFollowerCnt(),
		})
	}
	return items, resp.GetNextCursor(), nil
}

func (s *GRPCRelationService) Followers(ctx context.Context, userId, cursor int64, limit int32) ([]domain.RelationUser, int64, error) {
	resp, err := s.client.ListFollowers(ctx, &relationv1.ListRequest{Uid: userId, Cursor: cursor, Limit: limit})
	if err != nil {
		return nil, 0, err
	}
	edges := resp.GetEdges()
	ids := make([]int64, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, e.GetFollowerId())
	}
	users := s.resolveUsers(ctx, ids)
	states := s.relationStates(ctx, userId, ids) // viewer=列表主人：判是否已回关
	stats := s.batchStats(ctx, ids)
	items := make([]domain.RelationUser, 0, len(edges))
	for _, e := range edges {
		tid := e.GetFollowerId()
		u := users[tid]
		cnt := stats[tid]
		items = append(items, domain.RelationUser{
			Id:             tid,
			Name:           u.Nickname,
			Bio:            u.AboutMe,
			IsFollowedBack: states[tid].GetIsFollowing(),
			FolloweeCnt:    cnt.GetFolloweeCnt(),
			FollowerCnt:    cnt.GetFollowerCnt(),
			CreatedAt:      e.GetCreatedAt(),
		})
	}
	return items, resp.GetNextCursor(), nil
}

func (s *GRPCRelationService) Blocklist(ctx context.Context, uid, cursor int64, limit int32) ([]domain.RelationUser, int64, error) {
	resp, err := s.client.ListBlocks(ctx, &relationv1.ListRequest{Uid: uid, Cursor: cursor, Limit: limit})
	if err != nil {
		return nil, 0, err
	}
	edges := resp.GetEdges()
	ids := make([]int64, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, e.GetBlockedUid())
	}
	users := s.resolveUsers(ctx, ids)
	items := make([]domain.RelationUser, 0, len(edges))
	for _, e := range edges {
		tid := e.GetBlockedUid()
		u := users[tid]
		items = append(items, domain.RelationUser{
			Id:        tid,
			Name:      u.Nickname,
			Bio:       u.AboutMe,
			CreatedAt: e.GetCreatedAt(),
		})
	}
	return items, resp.GetNextCursor(), nil
}

func (s *GRPCRelationService) Stat(ctx context.Context, viewerId, userId int64) (domain.RelationStat, error) {
	statsResp, err := s.client.GetStats(ctx, &relationv1.GetStatsRequest{Uid: userId})
	if err != nil {
		return domain.RelationStat{}, err
	}
	st := statsResp.GetStats()
	out := domain.RelationStat{FolloweeCnt: st.GetFolloweeCnt(), FollowerCnt: st.GetFollowerCnt()}
	if viewerId > 0 && viewerId != userId {
		rs := s.relationStates(ctx, viewerId, []int64{userId})[userId]
		out.IsFollowing = rs.GetIsFollowing()
		out.IsMutual = rs.GetIsFollowing() && rs.GetIsFollowedBy()
		out.IsBlocked = rs.GetIsBlocked()
		out.IsBlockedBy = rs.GetIsBlockedBy()
	}
	return out, nil
}

// resolveUsers 批量解析 uid→用户（relation 只存 uid）。失败降级空 map（前端按首字母占位）。
func (s *GRPCRelationService) resolveUsers(ctx context.Context, ids []int64) map[int64]domain.User {
	if len(ids) == 0 {
		return map[int64]domain.User{}
	}
	users, err := s.userSvc.FindByIds(ctx, ids)
	if err != nil {
		s.l.Error("批量解析用户信息失败，降级", logger.Error(err))
		return map[int64]domain.User{}
	}
	return users
}

// relationStates 批量取 viewer 对 targetIds 的关系态；未登录/空则空 map。失败降级空 map（nil pb getter 安全返 false）。
func (s *GRPCRelationService) relationStates(ctx context.Context, viewerId int64, targetIds []int64) map[int64]*relationv1.RelationState {
	if viewerId <= 0 || len(targetIds) == 0 {
		return map[int64]*relationv1.RelationState{}
	}
	resp, err := s.client.GetRelation(ctx, &relationv1.GetRelationRequest{ViewerId: viewerId, TargetIds: targetIds})
	if err != nil {
		s.l.Error("获取关系态失败，降级", logger.Error(err))
		return map[int64]*relationv1.RelationState{}
	}
	return resp.GetStates()
}

// batchStats 批量取一批用户的关系聚合计数（列表卡展示每人 关注/粉丝 数）。失败降级空 map。
func (s *GRPCRelationService) batchStats(ctx context.Context, ids []int64) map[int64]*relationv1.RelationStats {
	if len(ids) == 0 {
		return map[int64]*relationv1.RelationStats{}
	}
	resp, err := s.client.BatchGetStats(ctx, &relationv1.BatchGetStatsRequest{Uids: ids})
	if err != nil {
		s.l.Error("批量获取关系计数失败，降级", logger.Error(err))
		return map[int64]*relationv1.RelationStats{}
	}
	return resp.GetStats()
}

// produce 关系事件 best-effort（changed 时调用）；失败只记日志不阻断写主流程。
func (s *GRPCRelationService) produce(ctx context.Context, typ string, followerId, followeeId int64) {
	if err := s.producer.Produce(ctx, relationevt.RelationEvent{
		Type: typ, FollowerId: followerId, FolloweeId: followeeId, Ts: time.Now().UnixMilli(),
	}); err != nil {
		s.l.Error("relation 事件生产失败",
			logger.String("type", typ), logger.Int64("followerId", followerId),
			logger.Int64("followeeId", followeeId), logger.Error(err))
	}
}
