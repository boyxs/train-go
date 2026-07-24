package repository

import (
	"context"

	"github.com/boyxs/train-go/webook/feed/domain"
	"github.com/boyxs/train-go/webook/feed/repository/cache"
)

// FeedRepository 关注流数据访问边界。
//
// 无 DAO：feed 的数据本质是「published_article × relation 的缓存投影」，源头分别在 core 与
// relation 库，直查别人库破坏服务边界。故本仓库只协调 cache（收件箱/发件箱全 Redis），
// 源数据由 service 层经 gRPC 拉取后交本仓库落缓存。丢失可重建，不需持久化兜底。
// 本层保持薄委托：分层规则要求 service 不直接碰 cache，故经此边界；无跨源聚合（那是 service 的活）。
type FeedRepository interface {
	InboxBuilt(ctx context.Context, uid int64) (bool, error)
	ReadInbox(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedItem, error)
	AppendInbox(ctx context.Context, uids []int64, item domain.FeedItem) error
	SaveInbox(ctx context.Context, uid int64, items []domain.FeedItem, bigvs []int64) error
	ReadBigv(ctx context.Context, uid int64) ([]int64, error)
	Invalidate(ctx context.Context, uids []int64) error
	InboxSince(ctx context.Context, uid, since int64, limit int) ([]int64, error)
	ReadOutbox(ctx context.Context, authorId, cursor int64, limit int) ([]domain.FeedItem, error)
	FillOutbox(ctx context.Context, authorId int64, items []domain.FeedItem) error
	AppendOutboxIfExists(ctx context.Context, authorId int64, item domain.FeedItem) error
	DelOutbox(ctx context.Context, authorId int64) error
}

type CacheFeedRepository struct {
	cache cache.FeedCache
}

func NewCacheFeedRepository(c cache.FeedCache) FeedRepository {
	return &CacheFeedRepository{cache: c}
}

func (r *CacheFeedRepository) InboxBuilt(ctx context.Context, uid int64) (bool, error) {
	return r.cache.InboxBuilt(ctx, uid)
}
func (r *CacheFeedRepository) ReadInbox(ctx context.Context, uid, cursor int64, limit int) ([]domain.FeedItem, error) {
	return r.cache.ReadInbox(ctx, uid, cursor, limit)
}
func (r *CacheFeedRepository) AppendInbox(ctx context.Context, uids []int64, item domain.FeedItem) error {
	return r.cache.AppendInbox(ctx, uids, item)
}
func (r *CacheFeedRepository) SaveInbox(ctx context.Context, uid int64, items []domain.FeedItem, bigvs []int64) error {
	return r.cache.SaveInbox(ctx, uid, items, bigvs)
}
func (r *CacheFeedRepository) ReadBigv(ctx context.Context, uid int64) ([]int64, error) {
	return r.cache.ReadBigv(ctx, uid)
}
func (r *CacheFeedRepository) Invalidate(ctx context.Context, uids []int64) error {
	return r.cache.Invalidate(ctx, uids)
}
func (r *CacheFeedRepository) InboxSince(ctx context.Context, uid, since int64, limit int) ([]int64, error) {
	return r.cache.InboxSince(ctx, uid, since, limit)
}
func (r *CacheFeedRepository) ReadOutbox(ctx context.Context, authorId, cursor int64, limit int) ([]domain.FeedItem, error) {
	return r.cache.ReadOutbox(ctx, authorId, cursor, limit)
}
func (r *CacheFeedRepository) FillOutbox(ctx context.Context, authorId int64, items []domain.FeedItem) error {
	return r.cache.FillOutbox(ctx, authorId, items)
}
func (r *CacheFeedRepository) AppendOutboxIfExists(ctx context.Context, authorId int64, item domain.FeedItem) error {
	return r.cache.AppendOutboxIfExists(ctx, authorId, item)
}
func (r *CacheFeedRepository) DelOutbox(ctx context.Context, authorId int64) error {
	return r.cache.DelOutbox(ctx, authorId)
}
