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

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	svcmocks "github.com/boyxs/train-go/webook/internal/service/mocks"
)

func TestSearchServer_SearchArticles_Happy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := svcmocks.NewMockArticleSearchService(ctrl)
	mockSvc.EXPECT().
		Search(gomock.Any(), "go", 1, 5).
		Return([]domain.Article{
			{Id: 11, Title: "Go 入门", Abstract: "讲 Go 的入门"},
			{Id: 12, Title: "Go 高级", Abstract: "讲 Go 的高级"},
		}, int64(2), nil)

	conn := startBufServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, NewSearchServer(mockSvc))
	})
	client := searchv1.NewSearchServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.SearchArticles(ctx, &searchv1.SearchArticlesRequest{
		Query: "go", Page: 1, Size: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.GetTotal())
	require.Len(t, resp.GetArticles(), 2)
	assert.Equal(t, int64(11), resp.GetArticles()[0].GetId())
	assert.Equal(t, "Go 入门", resp.GetArticles()[0].GetTitle())
	assert.Equal(t, "讲 Go 的入门", resp.GetArticles()[0].GetAbstract())
}

func TestSearchServer_SearchArticles_EmptyQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleSearchService(ctrl)
	// 期望：空 query 在 server 层就拦下，不应触达 svc.Search

	conn := startBufServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, NewSearchServer(mockSvc))
	})
	client := searchv1.NewSearchServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.SearchArticles(ctx, &searchv1.SearchArticlesRequest{
		Query: "  ", Page: 1, Size: 5,
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestSearchServer_SearchArticles_PropagateInternal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSvc := svcmocks.NewMockArticleSearchService(ctrl)
	mockSvc.EXPECT().
		Search(gomock.Any(), "x", 1, 5).
		Return(nil, int64(0), errors.New("es 故障"))

	conn := startBufServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, NewSearchServer(mockSvc))
	})
	client := searchv1.NewSearchServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.SearchArticles(ctx, &searchv1.SearchArticlesRequest{
		Query: "x", Page: 1, Size: 5,
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
