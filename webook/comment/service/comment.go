package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/boyxs/train-go/webook/comment/domain"
	"github.com/boyxs/train-go/webook/comment/errs"
	"github.com/boyxs/train-go/webook/comment/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/ratelimit"
	"github.com/boyxs/train-go/webook/pkg/sensitive"
)

const maxContentRunes = 500

type CommentService interface {
	Create(ctx context.Context, c domain.Comment) (domain.Comment, error)
	List(ctx context.Context, biz string, bizId int64, offset, limit int) ([]domain.Comment, error)
	GetReplies(ctx context.Context, rootId int64, offset, limit int) ([]domain.Comment, error)
	Delete(ctx context.Context, id, uid int64) error
	BatchGet(ctx context.Context, ids []int64) ([]domain.Comment, error)
	Count(ctx context.Context, biz string, bizId int64) (int64, error)
	BatchCount(ctx context.Context, biz string, bizIds []int64) (map[int64]int64, error)
}

type InternalCommentService struct {
	repo    repository.CommentRepository
	filter  sensitive.Filter
	limiter ratelimit.Limiter
	l       logger.LoggerX
}

func NewCommentService(repo repository.CommentRepository, filter sensitive.Filter, limiter ratelimit.Limiter, l logger.LoggerX) CommentService {
	return &InternalCommentService{repo: repo, filter: filter, limiter: limiter, l: l}
}

func (s *InternalCommentService) Create(ctx context.Context, c domain.Comment) (domain.Comment, error) {
	content := strings.TrimSpace(c.Content)
	if content == "" {
		return domain.Comment{}, errs.ErrContentEmpty
	}
	if utf8.RuneCountInString(content) > maxContentRunes {
		return domain.Comment{}, errs.ErrContentTooLong
	}
	if s.filter.Match(content) {
		return domain.Comment{}, errs.ErrSensitiveContent
	}
	// 按用户限流防刷；限流器自身故障时降级放行（可用性优先），仅记日志
	limited, err := s.limiter.Limit(ctx, fmt.Sprintf("comment:create:%d", c.UserId))
	if err != nil {
		s.l.Error(ctx, "评论限流器异常，降级放行", logger.Int64("uid", c.UserId), logger.Error(err))
	} else if limited {
		return domain.Comment{}, errs.ErrRateLimited
	}
	c.Content = content
	return s.repo.Create(ctx, c)
}

func (s *InternalCommentService) List(ctx context.Context, biz string, bizId int64, offset, limit int) ([]domain.Comment, error) {
	// 一级评论 reply_cnt 已在 DAO 维护为「整楼回复数」（写/删回复时增减楼根），直接返回即对齐展开条数
	return s.repo.PageRoots(ctx, biz, bizId, offset, limit)
}

func (s *InternalCommentService) GetReplies(ctx context.Context, rootId int64, offset, limit int) ([]domain.Comment, error) {
	return s.repo.GetReplies(ctx, rootId, offset, limit)
}

func (s *InternalCommentService) Delete(ctx context.Context, id, uid int64) error {
	ok, err := s.repo.Delete(ctx, id, uid)
	if err != nil {
		return err
	}
	if !ok {
		// repo.Delete 在非作者或评论不存在时返回 false
		return errs.ErrForbidden
	}
	return nil
}

// BatchGet 按 id 批量取（core 拿 interaction 热门 id 后回查详情）。
func (s *InternalCommentService) BatchGet(ctx context.Context, ids []int64) ([]domain.Comment, error) {
	return s.repo.BatchGet(ctx, ids)
}

func (s *InternalCommentService) Count(ctx context.Context, biz string, bizId int64) (int64, error) {
	return s.repo.Count(ctx, biz, bizId)
}

func (s *InternalCommentService) BatchCount(ctx context.Context, biz string, bizIds []int64) (map[int64]int64, error) {
	return s.repo.BatchCount(ctx, biz, bizIds)
}
