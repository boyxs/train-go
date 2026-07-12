package service

import (
	"context"
	"strings"
	"time"

	"github.com/boyxs/train-go/webook/tag/domain"
	"github.com/boyxs/train-go/webook/tag/errs"
	"github.com/boyxs/train-go/webook/tag/repository"
)

const (
	defaultSuggestLimit = 10
	maxSuggestLimit     = 20
	defaultBizWindow    = 200                // BizIdsByTag 默认候选窗口
	maxBizWindow        = 500                // 窗口封顶（候选窗口 ≤500，深翻页近似）
	weeklyNewWindow     = 7 * 24 * time.Hour // "本周新增" 滚动窗口
)

// TagService 通用标签业务：typeahead、打标签同步（校验/归一/去重/上限）、详情、批量、按标签取对象。
type TagService interface {
	Suggest(ctx context.Context, prefix string, limit int) ([]domain.Tag, error)
	// SyncTags 把 (biz,bizId) 的标签全量对齐到 names：校验(≤5/名合法) → 归一 slug → 去重 → 批量 Upsert → SyncBiz，返回已解析标签。
	SyncTags(ctx context.Context, biz string, bizId int64, names []string, source string) ([]domain.Tag, error)
	Detail(ctx context.Context, slug string) (domain.Tag, error)
	TagsBySlugs(ctx context.Context, slugs []string) ([]domain.Tag, error)
	TagsByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]domain.Tag, error)
	BizIdsByTag(ctx context.Context, slug, biz string, limit int) ([]int64, int64, error)
	// Follow / Unfollow uid 关注/取关 slug 标签（slug 不存在→ErrTagNotFound）；uid 由 core 鉴权后注入。
	// 返回是否真翻转（幂等：重复操作 changed=false）+ 翻转后的关注数。
	Follow(ctx context.Context, uid int64, slug string) (changed bool, followerCount int64, err error)
	Unfollow(ctx context.Context, uid int64, slug string) (changed bool, followerCount int64, err error)
	// IsFollowing viewer 是否正在关注该标签。
	IsFollowing(ctx context.Context, uid int64, slug string) (bool, error)
}

type InternalTagService struct {
	repo repository.TagRepository
}

func NewInternalTagService(repo repository.TagRepository) TagService {
	return &InternalTagService{repo: repo}
}

func (s *InternalTagService) Suggest(ctx context.Context, prefix string, limit int) ([]domain.Tag, error) {
	if limit <= 0 {
		limit = defaultSuggestLimit
	}
	if limit > maxSuggestLimit {
		limit = maxSuggestLimit
	}
	return s.repo.Suggest(ctx, strings.TrimSpace(prefix), limit)
}

func (s *InternalTagService) SyncTags(ctx context.Context, biz string, bizId int64, names []string, source string) ([]domain.Tag, error) {
	// 1. 归一 + 校验 + 按 slug 去重（校验早于任何写库，超限不落库）
	type normed struct{ name, slug string }
	seen := make(map[string]struct{}, len(names))
	norm := make([]normed, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !domain.IsValidTagName(name) {
			return nil, errs.ErrTagNameInvalid
		}
		slug := domain.NormalizeSlug(name)
		if slug == "" {
			return nil, errs.ErrTagNameInvalid
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		norm = append(norm, normed{name: name, slug: slug})
	}
	if len(norm) > domain.MaxTagsPerBiz {
		return nil, errs.ErrTagLimitExceeded
	}
	// 2. 批量 Upsert 解析出 tag id（一次 INSERT DoNothing + 一次回查，消 N+1）
	in := make([]domain.Tag, 0, len(norm))
	for _, n := range norm {
		in = append(in, domain.Tag{Name: n.name, Slug: n.slug, Type: domain.TagTypeTopic})
	}
	tags, err := s.repo.UpsertTags(ctx, in)
	if err != nil {
		return nil, err
	}
	tagIds := make([]int64, 0, len(tags))
	for _, t := range tags {
		tagIds = append(tagIds, t.Id)
	}
	// 3. 全量对齐关联（增删 + ref_count 在 repo/dao 事务内）
	if err := s.repo.SyncBiz(ctx, biz, bizId, tagIds, source); err != nil {
		return nil, err
	}
	return tags, nil
}

func (s *InternalTagService) Detail(ctx context.Context, slug string) (domain.Tag, error) {
	since := time.Now().Add(-weeklyNewWindow).UnixMilli()
	return s.repo.Detail(ctx, strings.TrimSpace(slug), since)
}

func (s *InternalTagService) TagsBySlugs(ctx context.Context, slugs []string) ([]domain.Tag, error) {
	return s.repo.TagsBySlugs(ctx, slugs)
}

func (s *InternalTagService) TagsByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]domain.Tag, error) {
	return s.repo.TagsByBiz(ctx, biz, bizIds)
}

func (s *InternalTagService) BizIdsByTag(ctx context.Context, slug, biz string, limit int) ([]int64, int64, error) {
	if limit <= 0 {
		limit = defaultBizWindow
	}
	if limit > maxBizWindow {
		limit = maxBizWindow
	}
	return s.repo.BizIdsByTag(ctx, strings.TrimSpace(slug), biz, limit)
}

func (s *InternalTagService) Follow(ctx context.Context, uid int64, slug string) (bool, int64, error) {
	return s.repo.Follow(ctx, uid, strings.TrimSpace(slug))
}

func (s *InternalTagService) Unfollow(ctx context.Context, uid int64, slug string) (bool, int64, error) {
	return s.repo.Unfollow(ctx, uid, strings.TrimSpace(slug))
}

func (s *InternalTagService) IsFollowing(ctx context.Context, uid int64, slug string) (bool, error) {
	return s.repo.IsFollowing(ctx, uid, strings.TrimSpace(slug))
}
