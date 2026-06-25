package ratelimit

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// 复用 grpc 内置 health proto 做真实 server/client，full method = /grpc.health.v1.Health/Check。
const (
	bufSize       = 1 << 16
	healthService = "grpc.health.v1.Health"
	healthMethod  = "/grpc.health.v1.Health/Check"
)

// fakeLimiter 记录最后一次被调的 key 与调用次数，断言"选中了哪个 limiter / 用了什么 key"。
type fakeLimiter struct {
	limited bool
	err     error
	lastKey string
	calls   int
}

func (f *fakeLimiter) Limit(_ context.Context, key string) (bool, error) {
	f.lastKey = key
	f.calls++
	return f.limited, f.err
}

// countingHealthServer 真实 handler：返回 SERVING 并计数，用于验证拦截器是否真的短路。
type countingHealthServer struct {
	healthpb.UnimplementedHealthServer
	calls int64
}

func (s *countingHealthServer) Check(_ context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	atomic.AddInt64(&s.calls, 1)
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

// dial 起 bufconn 真实 gRPC server（注册 health 服务 + 可选 server 拦截器），
// 返回带可选 client 拦截器的 HealthClient。
func dial(t *testing.T, hs healthpb.HealthServer, srvIc grpc.UnaryServerInterceptor, cliIc grpc.UnaryClientInterceptor) (healthpb.HealthClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)

	var srvOpts []grpc.ServerOption
	if srvIc != nil {
		srvOpts = append(srvOpts, grpc.UnaryInterceptor(srvIc))
	}
	srv := grpc.NewServer(srvOpts...)
	healthpb.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	dialOpts := []grpc.DialOption{
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if cliIc != nil {
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(cliIc))
	}
	conn, err := grpc.NewClient("passthrough://bufnet", dialOpts...)
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
	}
	return healthpb.NewHealthClient(conn), cleanup
}

func check(t *testing.T, client healthpb.HealthClient) (*healthpb.HealthCheckResponse, error) {
	t.Helper()
	return client.Check(context.Background(), &healthpb.HealthCheckRequest{})
}

// serviceOf 是纯解析，含 healthpb 端到端覆盖不到的边界（空串 / 无方法段），单独单测。
func TestServiceOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"标准全名", healthMethod, healthService},
		{"无前导斜杠", "webook.order.v1.OrderService/Create", "webook.order.v1.OrderService"},
		{"只有服务无方法", "/pkg.Service", "pkg.Service"},
		{"空串", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, serviceOf(tt.in))
		})
	}
}

func TestServer_UnderLimit_Serving(t *testing.T) {
	lim := &fakeLimiter{limited: false}
	b := NewInterceptorBuilder(lim, "ratelimit:global", nil)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	resp, err := check(t, client)
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
	assert.Equal(t, "ratelimit:global", lim.lastKey, "命中全局档，用全局 key")
}

func TestServer_OverLimit_ResourceExhausted(t *testing.T) {
	b := NewInterceptorBuilder(&fakeLimiter{limited: true}, "ratelimit:global", nil)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	_, err := check(t, client)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestServer_NoRule_PassesThrough(t *testing.T) {
	// 无全局 + 无规则 → 放行
	b := NewInterceptorBuilder(nil, "", nil)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	resp, err := check(t, client)
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
}

func TestServer_LimiterError_RejectsByDefault(t *testing.T) {
	b := NewInterceptorBuilder(&fakeLimiter{err: errors.New("redis down")}, "g", nil)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	_, err := check(t, client)
	assert.Equal(t, codes.Unavailable, status.Code(err), "默认保守：限流器故障即拒")
}

func TestServer_LimiterError_FailOpen(t *testing.T) {
	b := NewInterceptorBuilder(&fakeLimiter{err: errors.New("redis down")}, "g", nil).
		WithRejectOnErr(false)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	resp, err := check(t, client)
	require.NoError(t, err, "failOpen：限流器故障放行")
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
}

func TestServer_MethodOverridesService(t *testing.T) {
	methodLim := &fakeLimiter{limited: false} // 方法级放行
	svcLim := &fakeLimiter{limited: true}     // 服务级会限
	b := NewInterceptorBuilder(nil, "", nil).
		WithService(healthService, svcLim).
		WithMethod(healthMethod, methodLim)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	resp, err := check(t, client)
	require.NoError(t, err, "方法级最优先，放行")
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
	assert.Equal(t, keyPrefixMethod+healthMethod, methodLim.lastKey)
	assert.Zero(t, svcLim.calls, "命中方法级，服务级不应被调用")
}

func TestServer_ServiceLevel_Limits(t *testing.T) {
	svcLim := &fakeLimiter{limited: true}
	b := NewInterceptorBuilder(nil, "", nil).WithService(healthService, svcLim)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	_, err := check(t, client)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Equal(t, keyPrefixSvc+healthService, svcLim.lastKey)
}

func TestServer_WithMethods_RegistersMethod(t *testing.T) {
	lim := &fakeLimiter{limited: true}
	b := NewInterceptorBuilder(nil, "", nil).WithMethods([]string{healthMethod}, lim)
	client, cleanup := dial(t, &countingHealthServer{}, b.BuildUnaryServer(), nil)
	defer cleanup()

	_, err := check(t, client)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Equal(t, keyPrefixMethod+healthMethod, lim.lastKey)
}

func TestClient_OverLimit_ShortCircuits(t *testing.T) {
	hs := &countingHealthServer{}
	b := NewInterceptorBuilder(&fakeLimiter{limited: true}, "g", nil)
	client, cleanup := dial(t, hs, nil, b.BuildUnaryClient())
	defer cleanup()

	_, err := check(t, client)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Zero(t, atomic.LoadInt64(&hs.calls), "client 拦截器超限应短路，请求不到达 server")
}

func TestClient_UnderLimit_Invokes(t *testing.T) {
	hs := &countingHealthServer{}
	b := NewInterceptorBuilder(&fakeLimiter{limited: false}, "g", nil)
	client, cleanup := dial(t, hs, nil, b.BuildUnaryClient())
	defer cleanup()

	resp, err := check(t, client)
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
	assert.Equal(t, int64(1), atomic.LoadInt64(&hs.calls), "放行后 server 被调用一次")
}
