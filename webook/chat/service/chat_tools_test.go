package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	articlev1 "github.com/webook/api/gen/article/v1"
	interactionv1 "github.com/webook/api/gen/interaction/v1"
	searchv1 "github.com/webook/api/gen/search/v1"
	"github.com/webook/pkg/logger"
)

const bufSize = 1024 * 1024

// fakeSearch 可注入的 search 服务桩。
type fakeSearch struct {
	searchv1.UnimplementedSearchServiceServer
	fn func(req *searchv1.SearchArticlesRequest) (*searchv1.SearchArticlesResponse, error)
}

func (f *fakeSearch) SearchArticles(_ context.Context, req *searchv1.SearchArticlesRequest) (*searchv1.SearchArticlesResponse, error) {
	return f.fn(req)
}

type fakeArticle struct {
	articlev1.UnimplementedArticleReaderServiceServer
	fn      func(req *articlev1.GetArticleRequest) (*articlev1.Article, error)
	batchFn func(req *articlev1.BatchGetArticlesRequest) (*articlev1.BatchGetArticlesResponse, error)
}

func (f *fakeArticle) GetArticle(_ context.Context, req *articlev1.GetArticleRequest) (*articlev1.Article, error) {
	return f.fn(req)
}

func (f *fakeArticle) BatchGetArticles(_ context.Context, req *articlev1.BatchGetArticlesRequest) (*articlev1.BatchGetArticlesResponse, error) {
	return f.batchFn(req)
}

type fakeInteraction struct {
	interactionv1.UnimplementedInteractionServiceServer
	hotFn       func(req *interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error)
	collectedFn func(req *interactionv1.GetCollectedBizIdsRequest) (*interactionv1.GetCollectedBizIdsResponse, error)
}

func (f *fakeInteraction) GetHotBizIds(_ context.Context, req *interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error) {
	return f.hotFn(req)
}

func (f *fakeInteraction) GetCollectedBizIds(_ context.Context, req *interactionv1.GetCollectedBizIdsRequest) (*interactionv1.GetCollectedBizIdsResponse, error) {
	return f.collectedFn(req)
}

// startMockMonolith 起内存 gRPC server 模拟主仓，返回可直接喂给 ToolExecutor 的 3 个 client。
func startMockMonolith(t *testing.T, search *fakeSearch, article *fakeArticle, intr *fakeInteraction) (
	searchv1.SearchServiceClient,
	articlev1.ArticleReaderServiceClient,
	interactionv1.InteractionServiceClient,
) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	if search != nil {
		searchv1.RegisterSearchServiceServer(srv, search)
	} else {
		searchv1.RegisterSearchServiceServer(srv, &searchv1.UnimplementedSearchServiceServer{})
	}
	if article != nil {
		articlev1.RegisterArticleReaderServiceServer(srv, article)
	} else {
		articlev1.RegisterArticleReaderServiceServer(srv, &articlev1.UnimplementedArticleReaderServiceServer{})
	}
	if intr != nil {
		interactionv1.RegisterInteractionServiceServer(srv, intr)
	} else {
		interactionv1.RegisterInteractionServiceServer(srv, &interactionv1.UnimplementedInteractionServiceServer{})
	}

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("mock monolith exit: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
		srv.GracefulStop()
	})
	return searchv1.NewSearchServiceClient(conn),
		articlev1.NewArticleReaderServiceClient(conn),
		interactionv1.NewInteractionServiceClient(conn)
}

func newTestExecutor(t *testing.T, search *fakeSearch, article *fakeArticle, intr *fakeInteraction) ToolExecutor {
	searchCli, articleCli, intrCli := startMockMonolith(t, search, article, intr)
	zapLogger, _ := zap.NewDevelopment()
	return NewAIChatToolExecutor(searchCli, articleCli, intrCli, logger.NewZapLogger(zapLogger))
}

// ── search_articles ────────────────────────────────────────

func TestExecutor_SearchArticles_Happy(t *testing.T) {
	search := &fakeSearch{
		fn: func(req *searchv1.SearchArticlesRequest) (*searchv1.SearchArticlesResponse, error) {
			assert.Equal(t, "go", req.GetQuery())
			return &searchv1.SearchArticlesResponse{
				Articles: []*searchv1.ArticleCard{
					{Id: 11, Title: "Go 入门", Abstract: "讲 Go"},
					{Id: 12, Title: "Go 高级", Abstract: "讲 Go 进阶"},
				},
				Total: 2,
			}, nil
		},
	}
	exec := newTestExecutor(t, search, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, err := exec.Execute(ctx, 0, "search_articles", map[string]any{"query": "go"})
	require.NoError(t, err)
	require.Empty(t, r.Error)
	require.Len(t, r.Articles, 2)
	assert.Equal(t, int64(11), r.Articles[0].Id)
	assert.Equal(t, "Go 入门", r.Articles[0].Title)
	assert.Equal(t, "/article/11", r.Articles[0].Url)
}

func TestExecutor_SearchArticles_MissingQuery(t *testing.T) {
	exec := newTestExecutor(t, nil, nil, nil)

	r, err := exec.Execute(context.Background(), 0, "search_articles", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "缺少搜索关键词", r.Error)
}

func TestExecutor_SearchArticles_GRPCFailure(t *testing.T) {
	search := &fakeSearch{
		fn: func(_ *searchv1.SearchArticlesRequest) (*searchv1.SearchArticlesResponse, error) {
			return nil, status.Error(codes.Internal, "es 故障")
		},
	}
	exec := newTestExecutor(t, search, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := exec.Execute(ctx, 0, "search_articles", map[string]any{"query": "x"})
	require.NoError(t, err) // gRPC 错误降级为 ToolResultData{Error}，不向上抛
	assert.Equal(t, "搜索失败，请稍后重试", r.Error)
}

// ── get_hot_articles ───────────────────────────────────────

func TestExecutor_GetHotArticles_Happy(t *testing.T) {
	intr := &fakeInteraction{
		hotFn: func(req *interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error) {
			assert.Equal(t, "article", req.GetBiz())
			assert.Equal(t, int32(5), req.GetLimit())
			return &interactionv1.GetHotBizIdsResponse{BizIds: []int64{3, 1, 2}}, nil
		},
	}
	article := &fakeArticle{
		batchFn: func(req *articlev1.BatchGetArticlesRequest) (*articlev1.BatchGetArticlesResponse, error) {
			arts := make([]*articlev1.Article, 0, len(req.GetIds()))
			for _, id := range req.GetIds() {
				arts = append(arts, &articlev1.Article{Id: id, Title: "T", Abstract: "A"})
			}
			return &articlev1.BatchGetArticlesResponse{Articles: arts}, nil
		},
	}
	exec := newTestExecutor(t, nil, article, intr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := exec.Execute(ctx, 0, "get_hot_articles", map[string]any{})
	require.NoError(t, err)
	require.Empty(t, r.Error)
	require.Len(t, r.Articles, 3)
	assert.Equal(t, []int64{3, 1, 2}, []int64{r.Articles[0].Id, r.Articles[1].Id, r.Articles[2].Id})
}

func TestExecutor_GetHotArticles_NotFoundSilentSkip(t *testing.T) {
	intr := &fakeInteraction{
		hotFn: func(*interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error) {
			return &interactionv1.GetHotBizIdsResponse{BizIds: []int64{1, 2, 3}}, nil
		},
	}
	article := &fakeArticle{
		// 模拟 server 端 NotFound 静默过滤后返回的结果（id=2 不在）
		batchFn: func(req *articlev1.BatchGetArticlesRequest) (*articlev1.BatchGetArticlesResponse, error) {
			arts := make([]*articlev1.Article, 0, len(req.GetIds()))
			for _, id := range req.GetIds() {
				if id == 2 {
					continue
				}
				arts = append(arts, &articlev1.Article{Id: id, Title: "T", Abstract: "A"})
			}
			return &articlev1.BatchGetArticlesResponse{Articles: arts}, nil
		},
	}
	exec := newTestExecutor(t, nil, article, intr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := exec.Execute(ctx, 0, "get_hot_articles", map[string]any{})
	require.NoError(t, err)
	require.Len(t, r.Articles, 2)
	assert.Equal(t, []int64{1, 3}, []int64{r.Articles[0].Id, r.Articles[1].Id})
}

func TestExecutor_GetHotArticles_HotIdsFailure(t *testing.T) {
	intr := &fakeInteraction{
		hotFn: func(*interactionv1.GetHotBizIdsRequest) (*interactionv1.GetHotBizIdsResponse, error) {
			return nil, status.Error(codes.Unavailable, "down")
		},
	}
	exec := newTestExecutor(t, nil, nil, intr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := exec.Execute(ctx, 0, "get_hot_articles", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "获取热门文章失败", r.Error)
}

// ── get_my_favorites ───────────────────────────────────────

func TestExecutor_GetMyFavorites_Happy(t *testing.T) {
	intr := &fakeInteraction{
		collectedFn: func(req *interactionv1.GetCollectedBizIdsRequest) (*interactionv1.GetCollectedBizIdsResponse, error) {
			assert.Equal(t, int64(42), req.GetUid())
			return &interactionv1.GetCollectedBizIdsResponse{BizIds: []int64{8, 5}}, nil
		},
	}
	article := &fakeArticle{
		batchFn: func(req *articlev1.BatchGetArticlesRequest) (*articlev1.BatchGetArticlesResponse, error) {
			arts := make([]*articlev1.Article, 0, len(req.GetIds()))
			for _, id := range req.GetIds() {
				arts = append(arts, &articlev1.Article{Id: id, Title: "Fav", Abstract: "X"})
			}
			return &articlev1.BatchGetArticlesResponse{Articles: arts}, nil
		},
	}
	exec := newTestExecutor(t, nil, article, intr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := exec.Execute(ctx, 42, "get_my_favorites", map[string]any{})
	require.NoError(t, err)
	require.Empty(t, r.Error)
	require.Len(t, r.Articles, 2)
	assert.Equal(t, int64(8), r.Articles[0].Id)
}

// ── unknown tool / sanity ──────────────────────────────────

func TestExecutor_UnknownTool(t *testing.T) {
	exec := newTestExecutor(t, nil, nil, nil)
	r, err := exec.Execute(context.Background(), 0, "no_such_tool", map[string]any{})
	require.NoError(t, err)
	assert.Contains(t, r.Error, "未知工具")
}

func TestExecutor_Definitions(t *testing.T) {
	exec := newTestExecutor(t, nil, nil, nil)
	defs := exec.Definitions()
	require.Len(t, defs, 3)
	names := []string{defs[0].Name, defs[1].Name, defs[2].Name}
	assert.ElementsMatch(t,
		[]string{"search_articles", "get_hot_articles", "get_my_favorites"},
		names)
}

// 防御性：GRPC 错误类型断言可正确识别 NotFound
func TestExecutor_StatusError_TypeAssertion(t *testing.T) {
	st, ok := status.FromError(status.Error(codes.NotFound, "x"))
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.True(t, errors.Is(st.Err(), status.Error(codes.NotFound, "x")) ||
		status.Code(st.Err()) == codes.NotFound)
}
