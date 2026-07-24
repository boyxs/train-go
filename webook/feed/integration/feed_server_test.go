package integration

import (
	"context"
	"net"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	feedgrpc "github.com/boyxs/train-go/webook/feed/grpc"
	"github.com/boyxs/train-go/webook/feed/integration/setup"
	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
)

const bufSize = 1024 * 1024

// FeedServerSuite 真实 Redis + bufconn gRPC + fake relation/article：经 FeedServiceClient 发真实请求，
// 打通 gRPC → service → repository → cache → Redis 全链路。场景由 setup fake 桩固定（作者 7 / 粉丝 1001）。
type FeedServerSuite struct {
	suite.Suite
	cmd    redis.Cmdable
	conn   *grpc.ClientConn
	srv    *grpc.Server
	client feedv1.FeedServiceClient
}

func TestFeedServer(t *testing.T) {
	suite.Run(t, &FeedServerSuite{})
}

func (s *FeedServerSuite) SetupSuite() {
	s.cmd = setup.InitRedis()

	lis := bufconn.Listen(bufSize)
	s.srv = grpc.NewServer(grpc.ChainUnaryInterceptor(
		errconv.UnaryServerInterceptor(nil),
		feedgrpc.ValidateUnaryInterceptor,
	))
	feedv1.RegisterFeedServiceServer(s.srv, setup.InitFeedServer())
	go func() { _ = s.srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errconv.UnaryClientInterceptor()),
	)
	s.Require().NoError(err)
	s.conn = conn
	s.client = feedv1.NewFeedServiceClient(conn)
}

func (s *FeedServerSuite) TearDownSuite() {
	_ = s.conn.Close()
	s.srv.Stop()
}

// 每个用例前清理测试键，保证独立。
func (s *FeedServerSuite) SetupTest() {
	s.cmd.Del(context.Background(),
		"feed:inbox:1001", "feed:inbox:built:1001", "feed:bigv:1001", "feed:outbox:7")
}

// miss 重建 → 扩散 → 再读：完整读写链路
func (s *FeedServerSuite) TestRebuildThenFanout() {
	ctx := context.Background()

	// 首次读：收件箱未建 → rebuild 从源（fake article）拉作者 7 的文章 101
	resp, err := s.client.ListFeed(ctx, &feedv1.ListFeedRequest{Uid: 1001, Cursor: 0, Limit: 10})
	s.Require().NoError(err)
	s.Require().Len(resp.GetItems(), 1)
	s.Equal(int64(101), resp.GetItems()[0].GetArticleId())

	// 作者 7 发新文章 200 → 扩散给粉丝 1001
	_, err = s.client.FanoutArticle(ctx, &feedv1.FanoutArticleRequest{ArticleId: 200, AuthorId: 7, PublishedAt: 2000})
	s.Require().NoError(err)

	// 再读：built 已置 → 直接读收件箱，200（较新）在最前
	resp, err = s.client.ListFeed(ctx, &feedv1.ListFeedRequest{Uid: 1001, Cursor: 0, Limit: 10})
	s.Require().NoError(err)
	s.Require().Len(resp.GetItems(), 2)
	s.Equal(int64(200), resp.GetItems()[0].GetArticleId())
	s.Equal(int64(101), resp.GetItems()[1].GetArticleId())
	s.False(resp.GetHasMore())
}

// 校验拦截器：uid<=0 被拒
func (s *FeedServerSuite) TestValidate_BadUid() {
	_, err := s.client.ListFeed(context.Background(), &feedv1.ListFeedRequest{Uid: 0, Limit: 10})
	s.Error(err)
}

// 失效重建：Invalidate 后 built 标记被清
func (s *FeedServerSuite) TestInvalidate_ClearsBuilt() {
	ctx := context.Background()
	// 先读一次建收件箱（置 built）
	_, err := s.client.ListFeed(ctx, &feedv1.ListFeedRequest{Uid: 1001, Cursor: 0, Limit: 10})
	s.Require().NoError(err)
	n, err := s.cmd.Exists(ctx, "feed:inbox:built:1001").Result()
	s.Require().NoError(err)
	s.Require().Equal(int64(1), n, "读后应置 built")

	// 关系变更失效
	_, err = s.client.InvalidateInboxes(ctx, &feedv1.InvalidateInboxesRequest{Uids: []int64{1001}})
	s.Require().NoError(err)

	n, err = s.cmd.Exists(ctx, "feed:inbox:built:1001").Result()
	s.Require().NoError(err)
	s.Equal(int64(0), n, "失效后 built 应被清")
}
