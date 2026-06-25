package metrics

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1 << 16

type fakeHealthServer struct {
	healthpb.UnimplementedHealthServer
	err error
}

func (s *fakeHealthServer) Check(_ context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func newBuilder(reg prometheus.Registerer) *PrometheusBuilder {
	return NewPrometheusBuilder("webook", "grpc", "requests", "gRPC 请求").Registry(reg)
}

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

	dialOpts := []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if cliIc != nil {
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(cliIc))
	}
	conn, err := grpc.NewClient("passthrough://bufnet", dialOpts...)
	require.NoError(t, err)
	return healthpb.NewHealthClient(conn), func() { _ = conn.Close(); srv.Stop() }
}

func callCheck(t *testing.T, client healthpb.HealthClient, ctx context.Context) {
	t.Helper()
	_, _ = client.Check(ctx, &healthpb.HealthCheckRequest{})
}

func gatherText(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	var sb strings.Builder
	for _, mf := range mfs {
		sb.WriteString(mf.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

func gatherNames(t *testing.T, reg *prometheus.Registry) []string {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(mfs))
	for _, mf := range mfs {
		names = append(names, mf.GetName())
	}
	return names
}

func TestServerCounter_IncrementAndLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, newBuilder(reg).WithCounter().BuildUnaryServer(), nil)
	defer cleanup()

	// client 带上 app 头，server 侧 peer 标签应记录之
	ctx := metadata.AppendToOutgoingContext(context.Background(), "app", "chat")
	callCheck(t, client, ctx)

	assert.Equal(t, 1, testutil.CollectAndCount(reg, "webook_grpc_requests_total"))
	text := gatherText(t, reg)
	assert.Contains(t, text, `value:"server"`, "type 标签")
	assert.Contains(t, text, `value:"grpc.health.v1.Health"`, "service 标签")
	assert.Contains(t, text, `value:"Check"`, "method 标签")
	assert.Contains(t, text, `value:"chat"`, "peer 标签来自 app 头")
	assert.Contains(t, text, `value:"OK"`, "code 标签")
}

func TestServerCounter_ErrorCode(t *testing.T) {
	reg := prometheus.NewRegistry()
	hs := &fakeHealthServer{err: status.Error(codes.NotFound, "nope")}
	client, cleanup := dial(t, hs, newBuilder(reg).WithCounter().BuildUnaryServer(), nil)
	defer cleanup()

	callCheck(t, client, context.Background())
	assert.Contains(t, gatherText(t, reg), `value:"NotFound"`)
}

func TestHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, newBuilder(reg).WithHistogram().Buckets([]float64{0.123}).BuildUnaryServer(), nil)
	defer cleanup()

	callCheck(t, client, context.Background())
	text := gatherText(t, reg)
	assert.Contains(t, text, "webook_grpc_requests_duration_seconds")
	assert.Contains(t, text, "type:HISTOGRAM")
	assert.Contains(t, text, "sample_count:1")
	assert.Contains(t, text, "0.123", "自定义 Buckets 生效")
	// histogram 只带 type/service/method，控基数
	assert.Contains(t, text, `name:"type"`)
	assert.Contains(t, text, `name:"service"`)
	assert.Contains(t, text, `name:"method"`)
	assert.NotContains(t, text, `name:"peer"`, "histogram 不带 peer（控基数）")
	assert.NotContains(t, text, `name:"code"`, "histogram 不带 code")
}

func TestSummary(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, newBuilder(reg).WithSummary().Objectives(map[float64]float64{0.5: 0.05}).BuildUnaryServer(), nil)
	defer cleanup()

	callCheck(t, client, context.Background())
	text := gatherText(t, reg)
	assert.Contains(t, text, "webook_grpc_requests_duration_seconds_summary")
	assert.Contains(t, text, "type:SUMMARY")
}

func TestInFlight(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, newBuilder(reg).WithInFlight().BuildUnaryServer(), nil)
	defer cleanup()

	callCheck(t, client, context.Background())
	assert.Equal(t, 1, testutil.CollectAndCount(reg, "webook_grpc_requests_in_flight"))
}

func TestOnlyEnabledRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, newBuilder(reg).WithCounter().BuildUnaryServer(), nil)
	defer cleanup()

	callCheck(t, client, context.Background())
	names := gatherNames(t, reg)
	assert.Contains(t, names, "webook_grpc_requests_total")
	assert.NotContains(t, names, "webook_grpc_requests_duration_seconds")
	assert.NotContains(t, names, "webook_grpc_requests_in_flight")
}

func TestNoneEnabled(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, newBuilder(reg).BuildUnaryServer(), nil)
	defer cleanup()

	callCheck(t, client, context.Background())
	mfs, err := reg.Gather()
	require.NoError(t, err)
	assert.Empty(t, mfs, "没启用任何指标 → 不注册")
}

func TestSplitMethod(t *testing.T) {
	tests := []struct {
		in      string
		service string
		method  string
	}{
		{"/grpc.health.v1.Health/Check", "grpc.health.v1.Health", "Check"},
		{"noslash", "noslash", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		svc, m := splitMethod(tt.in)
		assert.Equal(t, tt.service, svc, tt.in)
		assert.Equal(t, tt.method, m, tt.in)
	}
}

func TestClientCounter_Type(t *testing.T) {
	reg := prometheus.NewRegistry()
	client, cleanup := dial(t, &fakeHealthServer{}, nil, newBuilder(reg).WithCounter().BuildUnaryClient())
	defer cleanup()

	callCheck(t, client, context.Background())
	assert.Equal(t, 1, testutil.CollectAndCount(reg, "webook_grpc_requests_total"))
	assert.Contains(t, gatherText(t, reg), `value:"client"`, "client 侧 type=client")
}
