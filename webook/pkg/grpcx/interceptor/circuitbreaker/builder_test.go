package circuitbreaker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeBreaker 实现 aegis CircuitBreaker：可控 Allow 返回，记录 Mark 调用次数。
type fakeBreaker struct {
	allowErr error
	success  int
	failed   int
}

func (f *fakeBreaker) Allow() error { return f.allowErr }
func (f *fakeBreaker) MarkSuccess() { f.success++ }
func (f *fakeBreaker) MarkFailed()  { f.failed++ }

func okHandler(called *bool) grpc.UnaryHandler {
	return func(context.Context, any) (any, error) { *called = true; return "ok", nil }
}

func TestServer_Closed_Success_MarksSuccess(t *testing.T) {
	fb := &fakeBreaker{}
	called := false
	resp, err := NewInterceptorBuilder(fb).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{}, okHandler(&called))
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, called)
	assert.Equal(t, 1, fb.success)
	assert.Zero(t, fb.failed)
}

func TestServer_Closed_DependencyError_MarksFailed(t *testing.T) {
	fb := &fakeBreaker{}
	_, err := NewInterceptorBuilder(fb).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{},
		func(context.Context, any) (any, error) { return nil, status.Error(codes.Unavailable, "down") })
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, fb.failed)
	assert.Zero(t, fb.success)
}

func TestServer_Closed_ClientError_MarksSuccess(t *testing.T) {
	fb := &fakeBreaker{}
	_, err := NewInterceptorBuilder(fb).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{},
		func(context.Context, any) (any, error) { return nil, status.Error(codes.NotFound, "nope") })
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, 1, fb.success, "客户端错误不算依赖故障，计成功")
	assert.Zero(t, fb.failed)
}

func TestServer_Open_FailFast(t *testing.T) {
	fb := &fakeBreaker{allowErr: errors.New("open")}
	called := false
	_, err := NewInterceptorBuilder(fb).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{}, okHandler(&called))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.False(t, called, "熔断打开不调用 handler")
	assert.Zero(t, fb.failed, "拒绝不计失败")
	assert.Zero(t, fb.success)
}

func TestClient_Open_FailFast(t *testing.T) {
	fb := &fakeBreaker{allowErr: errors.New("open")}
	called := false
	err := NewInterceptorBuilder(fb).BuildUnaryClient()(
		context.Background(), "/x.Y/Z", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			called = true
			return nil
		})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.False(t, called)
}

func TestClient_Closed_Success(t *testing.T) {
	fb := &fakeBreaker{}
	err := NewInterceptorBuilder(fb).BuildUnaryClient()(
		context.Background(), "/x.Y/Z", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, 1, fb.success)
}

func TestIsDependencyFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"Unavailable", status.Error(codes.Unavailable, ""), true},
		{"DeadlineExceeded", status.Error(codes.DeadlineExceeded, ""), true},
		{"Internal", status.Error(codes.Internal, ""), true},
		{"ResourceExhausted", status.Error(codes.ResourceExhausted, ""), true},
		{"NotFound（客户端错误）", status.Error(codes.NotFound, ""), false},
		{"InvalidArgument", status.Error(codes.InvalidArgument, ""), false},
		{"非 status error 视作 Unknown", errors.New("x"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDependencyFailure(tt.err))
		})
	}
}
