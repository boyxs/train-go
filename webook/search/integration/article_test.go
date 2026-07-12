package integration

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/search/errs"
	"github.com/boyxs/train-go/webook/search/integration/setup"
	"github.com/boyxs/train-go/webook/search/repository/dao"
)

const bufSize = 1024 * 1024

// SearchServerSuite 真实 ES + bufconn gRPC：经 SearchServiceClient 发真实请求，
// 打通 gRPC → service（stub embedder）→ repository → dao → ES 全链路。
// 文档经 gRPC IndexArticle 入库（service 内部 stub embed），向量按标题分 Go/Rust 两簇，保留 kNN 判别。
type SearchServerSuite struct {
	suite.Suite
	client *elasticsearch.TypedClient
	conn   *grpc.ClientConn
	srv    *grpc.Server
	sc     searchv1.SearchServiceClient
	index  string
}

func TestSearchServer(t *testing.T) {
	suite.Run(t, &SearchServerSuite{})
}

func (s *SearchServerSuite) SetupSuite() {
	s.client = setup.InitESClient()
	s.index = viper.GetString("data.es.index")

	lis := bufconn.Listen(bufSize)
	s.srv = grpc.NewServer(grpc.UnaryInterceptor(errconv.UnaryServerInterceptor(nil)))
	searchv1.RegisterSearchServiceServer(s.srv, setup.InitSearchServer())
	go func() { _ = s.srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errconv.UnaryClientInterceptor()),
	)
	require.NoError(s.T(), err)
	s.conn = conn
	s.sc = searchv1.NewSearchServiceClient(conn)
}

func (s *SearchServerSuite) TearDownSuite() {
	_ = s.conn.Close()
	s.srv.GracefulStop()
	// s.index 是别名，删物理索引（别名随之自动摘除）；ES 8+ destructive_requires_name 默认禁通配删，必须点名物理索引
	_, _ = s.client.Indices.Delete(s.index + dao.IndexVersionSuffix).Do(context.Background())
}

// SetupTest 每个用例前删物理索引再经 ensureIndex 重建（含 mapping + 别名）：server 的 DAO 走同一别名，透明。
func (s *SearchServerSuite) SetupTest() {
	// 删物理索引（别名随之自动摘除）
	_, _ = s.client.Indices.Delete(s.index + dao.IndexVersionSuffix).Do(context.Background())
	// 兜底删「与别名同名的遗留物理索引」（旧无别名版本可能留下，会与新别名撞名致 PutAlias 失败）；
	// 正常情况下 s.index 是别名、删索引会报错，被忽略，无副作用。
	_, _ = s.client.Indices.Delete(s.index).Do(context.Background())
	setup.InitArticleDAO(s.client, logger.NewNopLogger()) // 构造触发 ensureIndex 重建物理索引 + 别名
}

func (s *SearchServerSuite) refresh() {
	_, err := s.client.Indices.Refresh().Index(s.index).Do(context.Background())
	require.NoError(s.T(), err)
}

// indexDoc 经 gRPC IndexArticle 入库（service 内部 stub embed 标题 → 向量），不直灌 ES。
func (s *SearchServerSuite) indexDoc(doc *searchv1.ArticleDoc) {
	_, err := s.sc.IndexArticle(context.Background(), &searchv1.IndexArticleRequest{Doc: doc})
	require.NoError(s.T(), err)
}

func (s *SearchServerSuite) seedSamples() {
	docs := []*searchv1.ArticleDoc{
		{Id: 1, Title: "Go 并发编程", AuthorName: "张三", Status: 2, Category: "tech", Tags: []string{"golang", "并发"}, CreatedAt: 1000},
		{Id: 2, Title: "Go 性能优化", AuthorName: "李四", Status: 2, Category: "tech", Tags: []string{"golang", "性能"}, CreatedAt: 2000},
		{Id: 3, Title: "Rust 入门", AuthorName: "王五", Status: 2, Category: "tech", Tags: []string{"rust"}, CreatedAt: 3000},
		{Id: 4, Title: "Go 未发布草稿", AuthorName: "赵六", Status: 1, Category: "tech", Tags: []string{"golang"}, CreatedAt: 4000},
	}
	for _, d := range docs {
		s.indexDoc(d)
	}
	s.refresh()
}

func cardIds(resp *searchv1.SearchArticlesResponse) []int64 {
	ids := make([]int64, 0, len(resp.GetArticles()))
	for _, c := range resp.GetArticles() {
		ids = append(ids, c.GetId())
	}
	return ids
}

func facetOf(resp *searchv1.SearchArticlesResponse) map[string]int64 {
	m := make(map[string]int64, len(resp.GetFacets()))
	for _, f := range resp.GetFacets() {
		m[f.GetSlug()] = f.GetCount()
	}
	return m
}

func (s *SearchServerSuite) search(query string, tags []string) *searchv1.SearchArticlesResponse {
	resp, err := s.sc.SearchArticles(context.Background(), &searchv1.SearchArticlesRequest{
		Query: query, FilterTags: tags, Page: 1, Size: 10,
	})
	require.NoError(s.T(), err)
	return resp
}

// 校验：空 query / 超长 query（错误经 errconv 跨 gRPC 仍 errors.Is 命中 sentinel）
func (s *SearchServerSuite) TestSearch_Validation() {
	_, err := s.sc.SearchArticles(context.Background(), &searchv1.SearchArticlesRequest{Query: "   ", Page: 1, Size: 10})
	assert.ErrorIs(s.T(), err, errs.ErrSearchQueryEmpty)
	_, err = s.sc.SearchArticles(context.Background(), &searchv1.SearchArticlesRequest{Query: strings.Repeat("x", 257), Page: 1, Size: 10})
	assert.ErrorIs(s.T(), err, errs.ErrSearchQueryTooLong)
}

// 关键词命中（BM25）+ 状态过滤 + 标签 facet 聚合
func (s *SearchServerSuite) TestSearch_KeywordAndFacets() {
	s.seedSamples()
	resp := s.search("Go", nil)
	assert.ElementsMatch(s.T(), []int64{1, 2}, cardIds(resp), "命中含 go 的已发布文（草稿 4 被状态过滤，Rust 3 无 go）")
	assert.Equal(s.T(), int64(2), resp.GetTotal())
	fm := facetOf(resp)
	assert.Equal(s.T(), int64(2), fm["golang"], "facet: golang 2 篇")
	assert.Equal(s.T(), int64(1), fm["并发"])
	assert.Equal(s.T(), int64(1), fm["性能"])
	assert.Zero(s.T(), fm["rust"], "Rust 不在关键词命中集")
}

// 标签 facet 过滤（post_filter）：收窄 hits，facet 计数不受影响
func (s *SearchServerSuite) TestSearch_TagFilter() {
	s.seedSamples()
	resp := s.search("Go", []string{"并发"})
	assert.Equal(s.T(), []int64{1}, cardIds(resp), "叠加 并发 标签 → 仅文 1")
	assert.Equal(s.T(), int64(1), resp.GetTotal())
	assert.Equal(s.T(), int64(2), facetOf(resp)["golang"], "facet 仍基于关键词命中集（不受 post_filter 影响）")
}

// IndexArticle 写入后可检索；RemoveArticle 下架后不可检索
func (s *SearchServerSuite) TestIndexAndRemove() {
	s.indexDoc(&searchv1.ArticleDoc{Id: 9, Title: "Go 索引测试", Status: 2, Category: "tech", Tags: []string{"golang"}})
	s.refresh()
	assert.Contains(s.T(), cardIds(s.search("Go", nil)), int64(9))

	_, err := s.sc.RemoveArticle(context.Background(), &searchv1.RemoveArticleRequest{Id: 9})
	require.NoError(s.T(), err)
	s.refresh()
	assert.NotContains(s.T(), cardIds(s.search("Go", nil)), int64(9))
}

// RecommendTags：embed(title+content) → kNN 相似文章标签聚合（Go 簇 pos1 近，Rust 簇 pos2 远）
func (s *SearchServerSuite) TestRecommendTags() {
	s.seedSamples()
	resp, err := s.sc.RecommendTags(context.Background(), &searchv1.RecommendTagsRequest{
		Title: "Go 并发", Content: "goroutine channel", K: 2,
	})
	require.NoError(s.T(), err)
	fm := make(map[string]int64, len(resp.GetTags()))
	for _, t := range resp.GetTags() {
		fm[t.GetSlug()] = t.GetCount()
	}
	assert.GreaterOrEqual(s.T(), fm["golang"], int64(2), "最相似的文都含 golang")
	assert.Zero(s.T(), fm["rust"], "Rust（pos2）不在最近邻")
}

// 复现：kNN 无相似度阈值时，K 大于近邻簇大小会把不相关文章标签凑进来。
// Rust 查询(pos2)仅 1 篇 Rust 文，K=3 会补 2 篇 Go 文(cosine=0) → 不该出现 golang。
func (s *SearchServerSuite) TestRecommendTags_NoIrrelevant() {
	s.seedSamples() // 2 篇 Go(pos1) + 1 篇 Rust(pos2) 已发布
	resp, err := s.sc.RecommendTags(context.Background(), &searchv1.RecommendTagsRequest{
		Title: "Rust 系统编程", Content: "ownership borrow lifetime", K: 3,
	})
	require.NoError(s.T(), err)
	fm := make(map[string]int64, len(resp.GetTags()))
	for _, t := range resp.GetTags() {
		fm[t.GetSlug()] = t.GetCount()
	}
	assert.GreaterOrEqual(s.T(), fm["rust"], int64(1), "相似的 Rust 文标签应在")
	assert.Zero(s.T(), fm["golang"], "Go 文与 Rust 查询 cosine=0，不该被凑进推荐")
}

// Part B：索引期 embed 用正文（不止 title+abstract）——标题无 rust、正文含 rust 的文应落 Rust 簇。
// 老行为（只 embed title/abstract）下该文会落 Go 簇(pos1)、被 Rust 查询漏掉 → fm[rust]=0，此断言即失败。
func (s *SearchServerSuite) TestIndex_EmbedsContent() {
	s.indexDoc(&searchv1.ArticleDoc{Id: 30, Title: "系统编程实战", Status: 2, Category: "tech",
		Tags: []string{"rust"}, Content: "rust ownership borrow lifetime"})
	s.indexDoc(&searchv1.ArticleDoc{Id: 31, Title: "Go 并发", Status: 2, Category: "tech",
		Tags: []string{"golang"}, Content: "goroutine channel"})
	s.refresh()
	resp, err := s.sc.RecommendTags(context.Background(), &searchv1.RecommendTagsRequest{
		Title: "Rust", Content: "rust", K: 3,
	})
	require.NoError(s.T(), err)
	fm := make(map[string]int64, len(resp.GetTags()))
	for _, t := range resp.GetTags() {
		fm[t.GetSlug()] = t.GetCount()
	}
	assert.GreaterOrEqual(s.T(), fm["rust"], int64(1), "正文含 rust（标题无）经正文 embed 落 Rust 簇，被命中")
	assert.Zero(s.T(), fm["golang"], "Go 文 cosine=0，被阈值过滤")
}
