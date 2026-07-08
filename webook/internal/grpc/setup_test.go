package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/boyxs/train-go/webook/pkg/grpcx/interceptor/errconv"
)

const bufSize = 1024 * 1024

// startBufServer 起一个内存 grpc.Server，调用方传入 register 完成业务注册。
// 默认装上 grpcx error interceptor（与生产 wire 注册一致），
// 业务 handler return *errs.Error 后客户端 status.Code(err) 拿到对应业务 code。
// 返回 client 端的 *grpc.ClientConn，自动通过 t.Cleanup 释放。
func startBufServer(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(errconv.UnaryServerInterceptor(nil)),
	)
	register(srv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("grpc serve exit: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errconv.UnaryClientInterceptor()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		srv.GracefulStop()
	})

	return conn
}
