package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// recLogger 把每次日志的 fields 抓成 map，便于断言具体字段。
type recLogger struct {
	logs []map[string]any
	msgs []string
}

func (r *recLogger) record(msg string, fs []logger.Field) {
	m := make(map[string]any, len(fs))
	for _, f := range fs {
		m[f.Key] = f.Val
	}
	r.logs = append(r.logs, m)
	r.msgs = append(r.msgs, msg)
}
func (r *recLogger) Debug(msg string, fs ...logger.Field) { r.record(msg, fs) }
func (r *recLogger) Info(msg string, fs ...logger.Field)  { r.record(msg, fs) }
func (r *recLogger) Warn(msg string, fs ...logger.Field)  { r.record(msg, fs) }
func (r *recLogger) Error(msg string, fs ...logger.Field) { r.record(msg, fs) }

func (r *recLogger) WithContext(context.Context) logger.LoggerX { return r }

func (r *recLogger) last() map[string]any { return r.logs[len(r.logs)-1] }
func (r *recLogger) lastMsg() string      { return r.msgs[len(r.msgs)-1] }

func TestServer_Normal(t *testing.T) {
	rec := &recLogger{}
	_, err := NewInterceptorBuilder(rec).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { return "ok", nil })
	assert.NoError(t, err)

	assert.Equal(t, "Server RPC请求", rec.lastMsg())
	f := rec.last()
	assert.Equal(t, "normal", f["event"])
	assert.Equal(t, "/x.Y/Z", f["method"])
	assert.Equal(t, "unary", f["type"])
	assert.Contains(t, f, "cost")
	assert.NotContains(t, f, "code", "成功不记 code")
}

func TestServer_Error_RecordsCode(t *testing.T) {
	rec := &recLogger{}
	_, err := NewInterceptorBuilder(rec).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { return nil, status.Error(codes.NotFound, "nope") })
	assert.Equal(t, codes.NotFound, status.Code(err))

	f := rec.last()
	assert.Equal(t, "NotFound", f["code"])
	assert.Equal(t, "nope", f["message"])
	assert.Equal(t, "normal", f["event"])
}

func TestServer_Panic_GenericToClient_DetailToLog(t *testing.T) {
	rec := &recLogger{}
	_, err := NewInterceptorBuilder(rec).BuildUnaryServer()(
		context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"},
		func(context.Context, any) (any, error) { panic("boom secret") })

	// 回客户端：generic Internal，不泄漏 panic 细节
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, "internal error", status.Convert(err).Message())
	assert.NotContains(t, status.Convert(err).Message(), "boom secret")

	// 日志里：event=recover + panic 详情 + stack
	f := rec.last()
	assert.Equal(t, "recover", f["event"])
	assert.Contains(t, f["panic"], "boom secret")
	assert.Contains(t, f, "stack")
	assert.Equal(t, "Internal", f["code"])
}

func TestClient_Normal(t *testing.T) {
	rec := &recLogger{}
	err := NewInterceptorBuilder(rec).BuildUnaryClient()(
		context.Background(), "/x.Y/Z", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return nil })
	assert.NoError(t, err)

	assert.Equal(t, "Client RPC请求", rec.lastMsg())
	f := rec.last()
	assert.Equal(t, "/x.Y/Z", f["method"])
	assert.Equal(t, "normal", f["event"])
}

func TestClient_Error_RecordsCode(t *testing.T) {
	rec := &recLogger{}
	err := NewInterceptorBuilder(rec).BuildUnaryClient()(
		context.Background(), "/x.Y/Z", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return status.Error(codes.Unavailable, "down")
		})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, "Unavailable", rec.last()["code"])
}

func TestClient_Panic_Recovers(t *testing.T) {
	rec := &recLogger{}
	err := NewInterceptorBuilder(rec).BuildUnaryClient()(
		context.Background(), "/x.Y/Z", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { panic("boom") })

	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, "internal error", status.Convert(err).Message())
	f := rec.last()
	assert.Equal(t, "recover", f["event"])
	assert.Contains(t, f, "stack")
}
