package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	articlev1 "github.com/boyxs/train-go/webook/api/gen/article/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
)

func TestArticleReaderServer_GetArticle_Happy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	mockSvc.EXPECT().
		Detail(gomock.Any(), int64(7)).
		Return(domain.Article{Id: 7, Title: "Hello", Abstract: "测试"}, nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a, err := client.GetArticle(ctx, &articlev1.GetArticleRequest{Id: 7})
	require.NoError(t, err)
	assert.Equal(t, int64(7), a.GetId())
	assert.Equal(t, "Hello", a.GetTitle())
	assert.Equal(t, "测试", a.GetAbstract())
}

func TestArticleReaderServer_GetArticle_InvalidId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetArticle(ctx, &articlev1.GetArticleRequest{Id: 0})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestArticleReaderServer_GetArticle_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	mockSvc.EXPECT().
		Detail(gomock.Any(), int64(999)).
		Return(domain.Article{}, gorm.ErrRecordNotFound)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetArticle(ctx, &articlev1.GetArticleRequest{Id: 999})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestArticleReaderServer_BatchGetArticles_Happy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	mockSvc.EXPECT().BatchDetail(gomock.Any(), []int64{1, 2, 3}).Return([]domain.Article{
		{Id: 1, Title: "A", Abstract: "a"},
		{Id: 2, Title: "B", Abstract: "b"},
		{Id: 3, Title: "C", Abstract: "c"},
	}, nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.BatchGetArticles(ctx, &articlev1.BatchGetArticlesRequest{Ids: []int64{1, 2, 3}})
	require.NoError(t, err)
	require.Len(t, resp.GetArticles(), 3)
	got := []int64{resp.GetArticles()[0].GetId(), resp.GetArticles()[1].GetId(), resp.GetArticles()[2].GetId()}
	assert.ElementsMatch(t, []int64{1, 2, 3}, got)
}

func TestArticleReaderServer_BatchGetArticles_NotFoundSilentSkip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	// repository 层已对 NotFound 静默过滤，service 直接返回查到的 2 条
	mockSvc.EXPECT().BatchDetail(gomock.Any(), []int64{1, 2, 3}).Return([]domain.Article{
		{Id: 1, Title: "A", Abstract: "a"},
		{Id: 3, Title: "C", Abstract: "c"},
	}, nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.BatchGetArticles(ctx, &articlev1.BatchGetArticlesRequest{Ids: []int64{1, 2, 3}})
	require.NoError(t, err)
	require.Len(t, resp.GetArticles(), 2) // id=2 跳过
}

func TestArticleReaderServer_BatchGetArticles_EmptyIds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	// 期望：空 ids 不应触达 svc

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.BatchGetArticles(ctx, &articlev1.BatchGetArticlesRequest{Ids: nil})
	require.NoError(t, err)
	assert.Len(t, resp.GetArticles(), 0)
}

func TestArticleReaderServer_BatchGetArticles_TooMany(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tooMany := make([]int64, 101)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	_, err := client.BatchGetArticles(ctx, &articlev1.BatchGetArticlesRequest{Ids: tooMany})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestArticleReaderServer_ListAuthorArticles_Happy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	mockSvc.EXPECT().ListAuthorBriefs(gomock.Any(), int64(7), 100).Return([]domain.ArticleBrief{
		{Id: 201, PublishedAt: 2010},
		{Id: 101, PublishedAt: 1010},
	}, nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.ListAuthorArticles(ctx, &articlev1.ListAuthorArticlesRequest{AuthorId: 7, Limit: 100})
	require.NoError(t, err)
	require.Len(t, resp.GetItems(), 2)
	assert.Equal(t, int64(201), resp.GetItems()[0].GetId())
	assert.Equal(t, int64(2010), resp.GetItems()[0].GetPublishedAt())
	assert.Equal(t, int64(101), resp.GetItems()[1].GetId())
}

func TestArticleReaderServer_ListAuthorArticles_InvalidAuthorId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl) // 不应触达 svc

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.ListAuthorArticles(ctx, &articlev1.ListAuthorArticlesRequest{AuthorId: 0, Limit: 10})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestArticleReaderServer_GetArticle_Internal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleReaderService(ctrl)
	mockSvc.EXPECT().
		Detail(gomock.Any(), int64(1)).
		Return(domain.Article{}, errors.New("db down"))

	conn := startBufServer(t, func(s *grpc.Server) {
		articlev1.RegisterArticleReaderServiceServer(s, NewArticleReaderServer(mockSvc))
	})
	client := articlev1.NewArticleReaderServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetArticle(ctx, &articlev1.GetArticleRequest{Id: 1})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
