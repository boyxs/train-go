package repository

import (
	"context"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/repository/cache"
	"github.com/webook/internal/repository/dao"
	"github.com/webook/pkg/logger"
)

type ConversationRepository interface {
	Create(ctx context.Context, conv domain.Conversation) (domain.Conversation, error)
	List(ctx context.Context, uid int64) ([]domain.Conversation, error)
	Find(ctx context.Context, uid int64, convId int64) (domain.Conversation, error)
	UpdateTitle(ctx context.Context, uid int64, convId int64, title string) error
	Delete(ctx context.Context, uid int64, convId int64) error
}

type CacheConversationRepository struct {
	dao   dao.ConversationDAO
	cache cache.ConversationCache
	l     logger.LoggerX
}

func NewCacheConversationRepository(d dao.ConversationDAO, c cache.ConversationCache, l logger.LoggerX) ConversationRepository {
	return &CacheConversationRepository{dao: d, cache: c, l: l}
}

func (r *CacheConversationRepository) Create(ctx context.Context, conv domain.Conversation) (domain.Conversation, error) {
	entity, err := r.dao.Create(ctx, r.toEntity(conv))
	if err != nil {
		return domain.Conversation{}, err
	}
	r.delCache(ctx, conv.UserId)
	return r.toDomain(entity), nil
}

func (r *CacheConversationRepository) List(ctx context.Context, uid int64) ([]domain.Conversation, error) {
	// 缓存优先
	convs, err := r.cache.GetList(ctx, uid)
	if err == nil {
		return convs, nil
	}
	// 回源
	entities, err := r.dao.List(ctx, uid)
	if err != nil {
		return nil, err
	}
	convs = make([]domain.Conversation, 0, len(entities))
	for _, e := range entities {
		convs = append(convs, r.toDomain(e))
	}
	// 回填缓存（错误不阻塞主流程）
	if cacheErr := r.cache.SetList(ctx, uid, convs); cacheErr != nil {
		r.l.Error("回填对话列表缓存失败", logger.Int64("uid", uid), logger.Error(cacheErr))
	}
	return convs, nil
}

func (r *CacheConversationRepository) Find(ctx context.Context, uid int64, convId int64) (domain.Conversation, error) {
	entity, err := r.dao.Find(ctx, uid, convId)
	if err != nil {
		return domain.Conversation{}, err
	}
	return r.toDomain(entity), nil
}

func (r *CacheConversationRepository) UpdateTitle(ctx context.Context, uid int64, convId int64, title string) error {
	err := r.dao.UpdateTitle(ctx, uid, convId, title)
	if err == nil {
		r.delCache(ctx, uid)
	}
	return err
}

func (r *CacheConversationRepository) Delete(ctx context.Context, uid int64, convId int64) error {
	err := r.dao.Delete(ctx, uid, convId)
	if err == nil {
		r.delCache(ctx, uid)
	}
	return err
}

func (r *CacheConversationRepository) delCache(ctx context.Context, uid int64) {
	if err := r.cache.Del(ctx, uid); err != nil {
		r.l.Error("清除对话列表缓存失败", logger.Int64("uid", uid), logger.Error(err))
	}
}

func (r *CacheConversationRepository) toEntity(c domain.Conversation) dao.Conversation {
	return dao.Conversation{
		Id:     c.Id,
		UserId: c.UserId,
		Title:  c.Title,
	}
}

func (r *CacheConversationRepository) toDomain(c dao.Conversation) domain.Conversation {
	return domain.Conversation{
		Id:        c.Id,
		UserId:    c.UserId,
		Title:     c.Title,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
