package repository

import (
	"context"

	"github.com/webook/chat/domain"
	"github.com/webook/chat/repository/cache"
	"github.com/webook/chat/repository/dao"
	"github.com/webook/pkg/logger"
)

type MessageRepository interface {
	Insert(ctx context.Context, msg domain.Message) (domain.Message, error)
	UpdateContent(ctx context.Context, convId int64, id int64, content string, toolCalls string) error
	UpdateFeedback(ctx context.Context, convId int64, msgId int64, feedback int8) error
	ListRecent(ctx context.Context, convId int64, limit int) ([]domain.Message, error)
	// ListRecentLite 同 ListRecent 但不含 tool_calls，用于构建 prompt
	ListRecentLite(ctx context.Context, convId int64, limit int) ([]domain.Message, error)
	ListBefore(ctx context.Context, convId int64, beforeId int64, limit int) ([]domain.Message, error)
	ListAll(ctx context.Context, convId int64) ([]domain.Message, error)
	// Delete 软删除消息（清理生成中断残留的空占位行）
	Delete(ctx context.Context, convId int64, id int64) error
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
	// 缓存条数必须 >= limit，否则回源（buildPrompt 要 40 条，缓存可能只有 10 条）
	msgs, err := r.cache.GetList(ctx, convId)
	if err == nil && len(msgs) >= limit {
		return msgs[len(msgs)-limit:], nil
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

func (r *CacheMessageRepository) UpdateContent(ctx context.Context, convId int64, id int64, content string, toolCalls string) error {
	var tc *string
	if toolCalls != "" {
		tc = &toolCalls
	}
	if err := r.dao.Update(ctx, dao.Message{Id: id, Content: content, ToolCalls: tc}); err != nil {
		return err
	}
	// 写后清缓存（Cache-Aside），失败仅记日志：缓存最迟在 TTL 后失效，不阻断写流程
	if delErr := r.cache.Del(ctx, convId); delErr != nil {
		r.l.Error("清除消息缓存失败", logger.Int64("convId", convId), logger.Error(delErr))
	}
	return nil
}

func (r *CacheMessageRepository) UpdateFeedback(ctx context.Context, convId int64, msgId int64, feedback int8) error {
	err := r.dao.UpdateFeedback(ctx, msgId, convId, feedback)
	if err != nil {
		return err
	}
	if delErr := r.cache.Del(ctx, convId); delErr != nil {
		r.l.Error("清除消息缓存失败", logger.Int64("convId", convId), logger.Error(delErr))
	}
	return nil
}

func (r *CacheMessageRepository) ListRecentLite(ctx context.Context, convId int64, limit int) ([]domain.Message, error) {
	entities, err := r.dao.ListRecentLite(ctx, convId, limit)
	if err != nil {
		return nil, err
	}
	return r.toDomainSlice(entities), nil
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

func (r *CacheMessageRepository) Delete(ctx context.Context, convId int64, id int64) error {
	if err := r.dao.Delete(ctx, convId, id); err != nil {
		return err
	}
	// 写后清缓存（Cache-Aside），失败仅记日志
	if delErr := r.cache.Del(ctx, convId); delErr != nil {
		r.l.Error("清除消息缓存失败", logger.Int64("convId", convId), logger.Error(delErr))
	}
	return nil
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
		Feedback:       m.Feedback,
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
		Feedback:       m.Feedback,
		CreatedAt:      m.CreatedAt,
	}
}
