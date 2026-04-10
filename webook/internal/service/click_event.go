package service

import (
	"context"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
)

type ClickEventService interface {
	RecordClick(ctx context.Context, uid int64, articleId int64, convId int64, source string) error
	Dashboard(ctx context.Context) (domain.ClickEventDashboard, error)
}

type AIClickEventService struct {
	repo repository.ClickEventRepository
}

func NewAIClickEventService(repo repository.ClickEventRepository) ClickEventService {
	return &AIClickEventService{repo: repo}
}

func (s *AIClickEventService) RecordClick(ctx context.Context, uid int64, articleId int64, convId int64, source string) error {
	return s.repo.RecordClick(ctx, domain.ClickEvent{
		UserId:         uid,
		ArticleId:      articleId,
		ConversationId: convId,
		Source:         source,
	})
}

func (s *AIClickEventService) Dashboard(ctx context.Context) (domain.ClickEventDashboard, error) {
	return s.repo.Dashboard(ctx)
}
