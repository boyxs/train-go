package errconv

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/boyxs/train-go/webook/pkg/errs"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// 用例 5.x：bufconn 起真实 gRPC server + client，端到端验证双向转换。
//
// 复用 health/grpc_health_v1 这个标准 proto（已在 grpc-go 内置），
// 不引入业务 proto 依赖；server 端 handler 替换为我们要测的错误注入。

const bufSize = 1 << 16

type fakeHealthServer struct {
	healthpb.UnimplementedHealthServer
	wantErr error
}

func (s *fakeHealthServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if s.wantErr != nil {
		return nil, s.wantErr
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

// startBufServer 起一个 bufconn gRPC server，注入指定的 server 拦截器。
// 返回 client conn 和清理函数。
func startBufServer(t *testing.T, srvErr error, srvOpts ...grpc.ServerOption) (*grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(srvOpts...)
	healthpb.RegisterHealthServer(srv, &fakeHealthServer{wantErr: srvErr})
	go func() {
		_ = srv.Serve(lis)
	}()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientInterceptor()),
	)
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
	}
	return conn, cleanup
}

// 5.1 server: handler 返 *errs.Error{Code:404} → 客户端 status.Code(err)==NotFound
func TestUnaryServerInterceptor_BizError_ConvertsToStatus(t *testing.T) {
	be := errs.New(404, "用户不存在")
	conn, cleanup := startBufServer(t, be, grpc.UnaryInterceptor(UnaryServerInterceptor(nil)))
	defer cleanup()

	client := healthpb.NewHealthClient(conn)
	_, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.Error(t, err)

	// 客户端 interceptor 已 wrap 成 *errs.Error
	var got *errs.Error
	require.True(t, errors.As(err, &got))
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "用户不存在", got.Message)
}

// 5.2 server: handler 返 errors.New → 客户端拿到 codes.Internal
//
// 关键安全属性：原 err 内容不应泄漏到客户端（避免 SQL stmt / DSN / stack 等敏感信息出网）
func TestUnaryServerInterceptor_PlainError_ConvertsToInternal(t *testing.T) {
	const sensitive = "DSN=root:pass@tcp(internal-db:3306)/secret"
	conn, cleanup := startBufServer(t, errors.New(sensitive),
		grpc.UnaryInterceptor(UnaryServerInterceptor(nil)))
	defer cleanup()

	client := healthpb.NewHealthClient(conn)
	_, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.Error(t, err)

	var got *errs.Error
	require.True(t, errors.As(err, &got))
	assert.Equal(t, 500, got.Code)
	assert.NotContains(t, got.Message, "DSN", "敏感信息不应泄漏到客户端 message")
	assert.NotContains(t, got.Message, "secret", "敏感信息不应泄漏")
	assert.Equal(t, "internal error", got.Message, "应该是 generic message")
}

// 5.3 server: nil err 透传
func TestUnaryServerInterceptor_NilErr_PassesThrough(t *testing.T) {
	conn, cleanup := startBufServer(t, nil, grpc.UnaryInterceptor(UnaryServerInterceptor(nil)))
	defer cleanup()

	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
}

// 5.4 client: 收 status.Error(NotFound) → wrap 成 *errs.Error
// （server 不开 interceptor，直接抛 status.Error 模拟原生 gRPC server）
func TestUnaryClientInterceptor_StatusError_WrapsToBizError(t *testing.T) {
	conn, cleanup := startBufServer(t, status.Error(codes.NotFound, "not found"))
	defer cleanup()

	client := healthpb.NewHealthClient(conn)
	_, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.Error(t, err)

	var got *errs.Error
	require.True(t, errors.As(err, &got))
	assert.Equal(t, 404, got.Code)
	assert.Equal(t, "not found", got.Message)
}

// 5.5 全链路 round-trip：server 抛 *errs.Error → server interceptor 转 status →
//
//	客户端 client interceptor 转回 *errs.Error，Code/Message 无损
func TestRoundTrip_BizError_ServerToClient(t *testing.T) {
	original := errs.New(409, "邮箱已被注册")
	conn, cleanup := startBufServer(t, original, grpc.UnaryInterceptor(UnaryServerInterceptor(nil)))
	defer cleanup()

	client := healthpb.NewHealthClient(conn)
	_, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.Error(t, err)

	var got *errs.Error
	require.True(t, errors.As(err, &got))
	assert.Equal(t, original.Code, got.Code, "HTTP code 跨进程无损")
	assert.Equal(t, original.Message, got.Message, "Message 跨进程无损")
}

// 5.6 client: nil err 透传
func TestUnaryClientInterceptor_NilErr_PassesThrough(t *testing.T) {
	// 用 grpc-go 内置 health 真实 handler（不抛错）
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientInterceptor()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// recLogger 记录各级别调用次数，验证 UnaryServerInterceptor 的日志分级。
type recLogger struct {
	debugN, infoN, warnN, errorN int
}

func (r *recLogger) Debug(string, ...logger.Field) { r.debugN++ }
func (r *recLogger) Info(string, ...logger.Field)  { r.infoN++ }
func (r *recLogger) Warn(string, ...logger.Field)  { r.warnN++ }
func (r *recLogger) Error(string, ...logger.Field) { r.errorN++ }

func (r *recLogger) WithContext(context.Context) logger.LoggerX { return r }

// 5.7 客户端取消（context.Canceled）：降级 Debug 不刷 ERROR，回 codes.Canceled 而非 Internal。
func TestUnaryServerError_ClientCanceled_DebugNotError(t *testing.T) {
	rec := &recLogger{}
	intercept := UnaryServerInterceptor(rec)
	handler := func(_ context.Context, _ any) (any, error) {
		// 真实链路：向量化查询失败 -> do request -> context canceled
		return nil, fmt.Errorf("向量化查询失败: %w", context.Canceled)
	}
	_, err := intercept(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/webook.search.v1.SearchService/SearchArticles"}, handler)

	assert.Equal(t, codes.Canceled, status.Code(err), "客户端取消应回 Canceled，不是 Internal")
	assert.Equal(t, 0, rec.errorN, "客户端取消不应记 ERROR")
	assert.Equal(t, 1, rec.debugN, "客户端取消应记一条 Debug")
}

// 5.8 非取消的系统错误仍记 ERROR + 转 Internal（回归保护）。
func TestUnaryServerError_SystemError_StillError(t *testing.T) {
	rec := &recLogger{}
	intercept := UnaryServerInterceptor(rec)
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("db connection broken")
	}
	_, err := intercept(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/x"}, handler)

	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, 1, rec.errorN, "系统错误仍应记 ERROR")
	assert.Equal(t, 0, rec.debugN)
}
