package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/internal/domain"
	repomocks "github.com/boyxs/train-go/webook/internal/repository/mocks"
	"github.com/boyxs/train-go/webook/internal/service"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// 作者详情回显标签：Detail 经 tagSvc.TagsByBiz 补该文当前标签名，供编辑器预填。
func TestInternalArticleAuthorService_Detail_tags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authorRepo := repomocks.NewMockArticleAuthorRepository(ctrl)
	tagSvc := svcmocks.NewMockTagService(ctrl)

	const articleId, uid = int64(7), int64(9)
	authorRepo.EXPECT().FindById(gomock.Any(), articleId, uid).
		Return(domain.Article{Id: articleId, Title: "t"}, nil)
	tagSvc.EXPECT().
		TagsByBiz(gomock.Any(), domain.BizArticle, []int64{articleId}).
		Return(map[int64][]domain.Tag{
			articleId: {{Name: "Go"}, {Name: "泛型"}},
		}, nil)

	svc := service.NewInternalArticleAuthorService(
		authorRepo, nil, nil, tagSvc, logger.NewNopLogger(),
	)
	got, err := svc.Detail(context.Background(), articleId, uid)

	assert.NoError(t, err)
	assert.Equal(t, []string{"Go", "泛型"}, got.Tags)
}

// tag 服务失败：详情降级不带标签，主流程不失败。
func TestInternalArticleAuthorService_Detail_tagsDegrade(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authorRepo := repomocks.NewMockArticleAuthorRepository(ctrl)
	tagSvc := svcmocks.NewMockTagService(ctrl)

	const articleId, uid = int64(7), int64(9)
	authorRepo.EXPECT().FindById(gomock.Any(), articleId, uid).
		Return(domain.Article{Id: articleId, Title: "t"}, nil)
	tagSvc.EXPECT().
		TagsByBiz(gomock.Any(), domain.BizArticle, []int64{articleId}).
		Return(nil, assert.AnError)

	svc := service.NewInternalArticleAuthorService(
		authorRepo, nil, nil, tagSvc, logger.NewNopLogger(),
	)
	got, err := svc.Detail(context.Background(), articleId, uid)

	assert.NoError(t, err)
	assert.Empty(t, got.Tags)
}
