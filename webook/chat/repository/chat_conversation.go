package repository

import (
	"context"

	"github.com/boyxs/train-go/webook/chat/domain"
	"github.com/boyxs/train-go/webook/chat/repository/cache"
	"github.com/boyxs/train-go/webook/chat/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
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
	r.delListCache(ctx, conv.UserId)
	return r.toDomain(entity), nil
}

func (r *CacheConversationRepository) List(ctx context.Context, uid int64) ([]domain.Conversation, error) {
	convs, err := r.cache.GetList(ctx, uid)
	if err == nil {
		return convs, nil
	}
	entities, err := r.dao.List(ctx, uid)
	if err != nil {
		return nil, err
	}
	convs = make([]domain.Conversation, 0, len(entities))
	for _, e := range entities {
		convs = append(convs, r.toDomain(e))
	}
	if cacheErr := r.cache.SetList(ctx, uid, convs); cacheErr != nil {
		r.l.Error(ctx, "回填对话列表缓存失败", logger.Int64("uid", uid), logger.Error(cacheErr))
	}
	return convs, nil
}

// Find 高频路径（service.SendMessage / SetFeedback / IsGenerating / ResumeStream 都调用），
// 走 Cache-Aside 单条缓存：cache key 含 uid，越权访问天然 miss 后由 DAO 二次校验返回 NotFound。
func (r *CacheConversationRepository) Find(ctx context.Context, uid int64, convId int64) (domain.Conversation, error) {
	if conv, err := r.cache.Get(ctx, uid, convId); err == nil {
		return conv, nil
	}
	entity, err := r.dao.Find(ctx, uid, convId)
	if err != nil {
		return domain.Conversation{}, err
	}
	conv := r.toDomain(entity)
	if setErr := r.cache.Set(ctx, conv); setErr != nil {
		r.l.Error(ctx, "回填单条对话缓存失败",
			logger.Int64("uid", uid), logger.Int64("convId", convId), logger.Error(setErr))
	}
	return conv, nil
}

func (r *CacheConversationRepository) UpdateTitle(ctx context.Context, uid int64, convId int64, title string) error {
	err := r.dao.UpdateTitle(ctx, uid, convId, title)
	if err == nil {
		r.delListCache(ctx, uid)
		r.delItemCache(ctx, uid, convId)
	}
	return err
}

func (r *CacheConversationRepository) Delete(ctx context.Context, uid int64, convId int64) error {
	err := r.dao.Delete(ctx, uid, convId)
	if err == nil {
		r.delListCache(ctx, uid)
		r.delItemCache(ctx, uid, convId)
	}
	return err
}

func (r *CacheConversationRepository) delListCache(ctx context.Context, uid int64) {
	if err := r.cache.Del(ctx, uid); err != nil {
		r.l.Error(ctx, "清除对话列表缓存失败", logger.Int64("uid", uid), logger.Error(err))
	}
}

func (r *CacheConversationRepository) delItemCache(ctx context.Context, uid int64, convId int64) {
	if err := r.cache.DelOne(ctx, uid, convId); err != nil {
		r.l.Error(ctx, "清除单条对话缓存失败",
			logger.Int64("uid", uid), logger.Int64("convId", convId), logger.Error(err))
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
