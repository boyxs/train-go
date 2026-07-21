package repository

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/slicex"
	"github.com/boyxs/train-go/webook/tag/domain"
	"github.com/boyxs/train-go/webook/tag/errs"
	"github.com/boyxs/train-go/webook/tag/repository/cache"
	"github.com/boyxs/train-go/webook/tag/repository/dao"
)

// TagRepository 协调 tag/tagging/tag_follow 三 DAO，做 domain↔dao 转换。详情热点读走 Cache-Aside。
type TagRepository interface {
	UpsertTags(ctx context.Context, tags []domain.Tag) ([]domain.Tag, error)
	SyncBiz(ctx context.Context, biz string, bizId int64, tagIds []int64, source string) error
	// Detail 标签详情：解析 slug→tag + 填充 since 以来的新增关联数（WeeklyNewCount）。缺失→ErrTagNotFound。
	Detail(ctx context.Context, slug string, since int64) (domain.Tag, error)
	Suggest(ctx context.Context, prefix string, limit int) ([]domain.Tag, error)
	TagsBySlugs(ctx context.Context, slugs []string) ([]domain.Tag, error)
	// TagsByBiz 批量取多个对象各自的标签（列表页消 N+1）：tagging.BatchByBiz → tag.FindByIds 组装。
	TagsByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]domain.Tag, error)
	// BizIdsByTag 解析 slug→tagId 后取该标签下某 biz 的对象 id 窗口 + total；slug 不存在返回空。
	BizIdsByTag(ctx context.Context, slug, biz string, limit int) ([]int64, int64, error)
	// Follow / Unfollow 解析 slug→tag（不存在→ErrTagNotFound）后翻转关注边，返回是否真翻转 + 翻转后关注数。
	Follow(ctx context.Context, uid int64, slug string) (changed bool, followerCount int64, err error)
	Unfollow(ctx context.Context, uid int64, slug string) (changed bool, followerCount int64, err error)
	// IsFollowing viewer 是否正在关注该标签；slug 不存在→ErrTagNotFound。
	IsFollowing(ctx context.Context, uid int64, slug string) (bool, error)
}

type InternalTagRepository struct {
	tagDAO     dao.TagDAO
	taggingDAO dao.TaggingDAO
	followDAO  dao.TagFollowDAO
	cache      cache.TagCache
	l          logger.LoggerX
}

func NewInternalTagRepository(tagDAO dao.TagDAO, taggingDAO dao.TaggingDAO, followDAO dao.TagFollowDAO, c cache.TagCache, l logger.LoggerX) TagRepository {
	return &InternalTagRepository{tagDAO: tagDAO, taggingDAO: taggingDAO, followDAO: followDAO, cache: c, l: l}
}

// delDetail 写路径失效详情缓存：失败仅记日志不阻断（主写已成功，脏缓存至 TTL）。
func (r *InternalTagRepository) delDetail(ctx context.Context, slugs ...string) {
	if err := r.cache.DelDetail(ctx, slugs...); err != nil {
		r.l.WithContext(ctx).Warn("失效标签详情缓存失败", logger.Error(err))
	}
}

func (r *InternalTagRepository) UpsertTags(ctx context.Context, tags []domain.Tag) ([]domain.Tag, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	list, err := r.tagDAO.UpsertTags(ctx, slicex.Map(tags, r.toEntity))
	if err != nil {
		return nil, err
	}
	// 回查结果为 DB 顺序，按输入 slug 顺序重排，保持返回顺序=入参顺序（调用方语义稳定）
	bySlug := make(map[string]domain.Tag, len(list))
	for _, t := range list {
		bySlug[t.Slug] = r.toDomain(t)
	}
	res := make([]domain.Tag, 0, len(tags))
	for _, in := range tags {
		if dt, ok := bySlug[in.Slug]; ok {
			res = append(res, dt)
		}
	}
	return res, nil
}

func (r *InternalTagRepository) SyncBiz(ctx context.Context, biz string, bizId int64, tagIds []int64, source string) error {
	affected, err := r.taggingDAO.SyncByBiz(ctx, biz, bizId, tagIds, source)
	if err != nil {
		return err
	}
	// 精确失效 ref_count 变化的标签（新增 ∪ 删除，含清空关联时的被移除标签）详情缓存
	if len(affected) > 0 {
		tags, err := r.tagDAO.FindByIds(ctx, affected)
		if err != nil {
			r.l.WithContext(ctx).Warn("失效标签详情缓存：解析 slug 失败", logger.Error(err))
			return nil // 主写已成功，缓存失效失败不回滚
		}
		slugs := make([]string, 0, len(tags))
		for _, t := range tags {
			slugs = append(slugs, t.Slug)
		}
		r.delDetail(ctx, slugs...)
	}
	return nil
}

func (r *InternalTagRepository) Detail(ctx context.Context, slug string, since int64) (domain.Tag, error) {
	cached, err := r.cache.GetDetail(ctx, slug)
	if err == nil {
		return cached, nil // 命中
	}
	if !errors.Is(err, redis.Nil) {
		// 非 miss 的真实故障（连接失败/值损坏）：记日志留观测信号，仍回源 DB
		r.l.WithContext(ctx).Warn("读标签详情缓存故障，回源 DB", logger.String("slug", slug), logger.Error(err))
	}
	// miss(redis.Nil) 或缓存故障 → 回源 DB
	t, err := r.findBySlug(ctx, slug)
	if err != nil {
		return domain.Tag{}, err
	}
	recent, err := r.taggingDAO.CountRecentByTag(ctx, t.Id, since)
	if err != nil {
		return domain.Tag{}, err
	}
	dt := r.toDomain(t)
	dt.WeeklyNewCount = recent
	if err := r.cache.SetDetail(ctx, dt); err != nil {
		r.l.WithContext(ctx).Warn("回填标签详情缓存失败", logger.String("slug", slug), logger.Error(err))
	}
	return dt, nil
}

// findBySlug 解析 slug→tag 本体，缺失返回 ErrTagNotFound（Detail/Follow/IsFollowing 共用）。
func (r *InternalTagRepository) findBySlug(ctx context.Context, slug string) (dao.Tag, error) {
	list, err := r.tagDAO.FindBySlugs(ctx, []string{slug})
	if err != nil {
		return dao.Tag{}, err
	}
	if len(list) == 0 {
		return dao.Tag{}, errs.ErrTagNotFound
	}
	return list[0], nil
}

func (r *InternalTagRepository) Follow(ctx context.Context, uid int64, slug string) (bool, int64, error) {
	t, err := r.findBySlug(ctx, slug)
	if err != nil {
		return false, 0, err
	}
	changed, cnt, err := r.followDAO.Follow(ctx, uid, t.Id)
	if err != nil {
		return false, 0, err
	}
	if changed { // followCount 变了才需失效（幂等重复关注不动缓存）
		r.delDetail(ctx, slug)
	}
	return changed, cnt, nil
}

func (r *InternalTagRepository) Unfollow(ctx context.Context, uid int64, slug string) (bool, int64, error) {
	t, err := r.findBySlug(ctx, slug)
	if err != nil {
		return false, 0, err
	}
	changed, cnt, err := r.followDAO.Unfollow(ctx, uid, t.Id)
	if err != nil {
		return false, 0, err
	}
	if changed {
		r.delDetail(ctx, slug)
	}
	return changed, cnt, nil
}

func (r *InternalTagRepository) IsFollowing(ctx context.Context, uid int64, slug string) (bool, error) {
	t, err := r.findBySlug(ctx, slug)
	if err != nil {
		return false, err
	}
	return r.followDAO.IsFollowing(ctx, uid, t.Id)
}

func (r *InternalTagRepository) Suggest(ctx context.Context, prefix string, limit int) ([]domain.Tag, error) {
	list, err := r.tagDAO.Suggest(ctx, prefix, limit)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomain), nil
}

func (r *InternalTagRepository) TagsBySlugs(ctx context.Context, slugs []string) ([]domain.Tag, error) {
	list, err := r.tagDAO.FindBySlugs(ctx, slugs)
	if err != nil {
		return nil, err
	}
	return slicex.Map(list, r.toDomain), nil
}

func (r *InternalTagRepository) TagsByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]domain.Tag, error) {
	taggingMap, err := r.taggingDAO.BatchByBiz(ctx, biz, bizIds)
	if err != nil {
		return nil, err
	}
	idSet := make(map[int64]struct{})
	for _, tgs := range taggingMap {
		for _, tg := range tgs {
			idSet[tg.TagId] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	tags, err := r.tagDAO.FindByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	tagById := make(map[int64]domain.Tag, len(tags))
	for _, t := range tags {
		tagById[t.Id] = r.toDomain(t)
	}
	res := make(map[int64][]domain.Tag, len(taggingMap))
	for bizId, tgs := range taggingMap {
		for _, tg := range tgs {
			if dt, ok := tagById[tg.TagId]; ok {
				res[bizId] = append(res[bizId], dt)
			}
		}
	}
	return res, nil
}

func (r *InternalTagRepository) BizIdsByTag(ctx context.Context, slug, biz string, limit int) ([]int64, int64, error) {
	list, err := r.tagDAO.FindBySlugs(ctx, []string{slug})
	if err != nil {
		return nil, 0, err
	}
	if len(list) == 0 {
		return nil, 0, nil // slug 不存在 → 空列表（非错误，标签页展示空态）
	}
	return r.taggingDAO.BizIdsByTag(ctx, list[0].Id, biz, limit)
}

func (r *InternalTagRepository) toDomain(t dao.Tag) domain.Tag {
	return domain.Tag{
		Id:          t.Id,
		Name:        t.Name,
		Slug:        t.Slug,
		Type:        t.Type,
		Description: t.Description,
		RefCount:    t.RefCount,
		FollowCount: t.FollowCount,
	}
}

func (r *InternalTagRepository) toEntity(t domain.Tag) dao.Tag {
	return dao.Tag{
		Id:          t.Id,
		Name:        t.Name,
		Slug:        t.Slug,
		Type:        t.Type,
		Description: t.Description,
		RefCount:    t.RefCount,
		FollowCount: t.FollowCount,
	}
}
