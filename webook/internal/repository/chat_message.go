package repository

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

type MessageRepository interface {
	Insert(ctx context.Context, msg domain.Message) (domain.Message, error)
	ListRecent(ctx context.Context, convId int64, limit int) ([]domain.Message, error)
	ListBefore(ctx context.Context, convId int64, beforeId int64, limit int) ([]domain.Message, error)
	ListAll(ctx context.Context, convId int64) ([]domain.Message, error)
}

type CacheMessageRepository struct {
	dao   dao.MessageDAO
	cache cache.MessageCache
	l     logger.LoggerX
}

func NewCacheMessageRepository(d dao.MessageDAO, c cache.MessageCache, l logger.LoggerX) MessageRepository {
	return &CacheMessageRepository{dao: d, cache: c, l: l}
}

func (r *CacheMessageRepository) Insert(ctx context.Context, msg domain.Message) (domain.Message, error) {
	entity, err := r.dao.Insert(ctx, r.toEntity(msg))
	if err != nil {
		return domain.Message{}, err
	}
	if delErr := r.cache.Del(ctx, msg.ConversationId); delErr != nil {
		r.l.Error("清除消息列表缓存失败", logger.Int64("convId", msg.ConversationId), logger.Error(delErr))
	}
	return r.toDomain(entity), nil
}

func (r *CacheMessageRepository) ListRecent(ctx context.Context, convId int64, limit int) ([]domain.Message, error) {
	// Cache-Aside：最新消息热路径，优先走缓存
	msgs, err := r.cache.GetList(ctx, convId)
	if err == nil && len(msgs) > 0 {
		if len(msgs) > limit {
			msgs = msgs[len(msgs)-limit:]
		}
		return msgs, nil
	}

	entities, err := r.dao.ListRecent(ctx, convId, limit)
	if err != nil {
		return nil, err
	}
	result := r.toDomainSlice(entities)

	// 回填缓存，失败不阻塞
	if setErr := r.cache.SetList(ctx, convId, result); setErr != nil {
		r.l.Error("回填消息缓存失败", logger.Int64("convId", convId), logger.Error(setErr))
	}
	return result, nil
}

func (r *CacheMessageRepository) ListBefore(ctx context.Context, convId int64, beforeId int64, limit int) ([]domain.Message, error) {
	entities, err := r.dao.ListBefore(ctx, convId, beforeId, limit)
	if err != nil {
		return nil, err
	}
	return r.toDomainSlice(entities), nil
}

func (r *CacheMessageRepository) ListAll(ctx context.Context, convId int64) ([]domain.Message, error) {
	entities, err := r.dao.ListAll(ctx, convId)
	if err != nil {
		return nil, err
	}
	return r.toDomainSlice(entities), nil
}

func (r *CacheMessageRepository) toDomainSlice(entities []dao.Message) []domain.Message {
	msgs := make([]domain.Message, 0, len(entities))
	for _, e := range entities {
		msgs = append(msgs, r.toDomain(e))
	}
	return msgs
}

func (r *CacheMessageRepository) toEntity(m domain.Message) dao.Message {
	var tc *string
	if m.ToolCalls != "" {
		tc = &m.ToolCalls
	}
	return dao.Message{
		Id:             m.Id,
		ConversationId: m.ConversationId,
		Role:           m.Role,
		Content:        m.Content,
		ToolCalls:      tc,
		TokenUsed:      m.TokenUsed,
	}
}

func (r *CacheMessageRepository) toDomain(m dao.Message) domain.Message {
	var tc string
	if m.ToolCalls != nil {
		tc = *m.ToolCalls
	}
	return domain.Message{
		Id:             m.Id,
		ConversationId: m.ConversationId,
		Role:           m.Role,
		Content:        m.Content,
		ToolCalls:      tc,
		TokenUsed:      m.TokenUsed,
		CreatedAt:      m.CreatedAt,
	}
}
