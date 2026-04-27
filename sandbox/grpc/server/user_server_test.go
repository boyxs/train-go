package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	userv1 "webook/sandbox/grpc/gen/user/v1"
)

const bufSize = 1024 * 1024

// startBufServer 起一个内存 gRPC server，client 连上后即可调用。
// 通过 t.Cleanup 自动注销，调用方无需手动 defer。
func startBufServer(t *testing.T) userv1.UserServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, NewMemoryUserServer())

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
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		srv.GracefulStop()
	})

	return userv1.NewUserServiceClient(conn)
}

func TestMemoryUserServer_GetUser_Found_EmailContact(t *testing.T) {
	client := startBufServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	require.NoError(t, err)
	require.NotNil(t, u)

	assert.Equal(t, int64(1), u.GetId())
	assert.Equal(t, "Alice", u.GetName())
	assert.Equal(t, "https://cdn.example.com/a.png", u.GetAvatar())
	assert.Equal(t, int32(28), u.GetAge())
	assert.Equal(t, "admin", u.GetAttributes()["role"])
	assert.ElementsMatch(t, []string{"Ally", "Lis"}, u.GetNicknames())
	assert.Equal(t, "Shenzhen", u.GetAddress().GetCity())
	assert.Equal(t, userv1.Gender_GENDER_FEMALE, u.GetGender())
	assert.Equal(t, "alice@example.com", u.GetEmail())
}

func TestMemoryUserServer_GetUser_Found_PhoneContact(t *testing.T) {
	client := startBufServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 2})
	require.NoError(t, err)

	_, ok := u.GetContacts().(*userv1.User_Phone)
	assert.True(t, ok, "expect oneof = Phone, got %T", u.GetContacts())
	assert.Equal(t, "13800000000", u.GetPhone())
}

func TestMemoryUserServer_GetUser_NotFound(t *testing.T) {
	client := startBufServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 999})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestMemoryUserServer_GetUser_InvalidArgument(t *testing.T) {
	client := startBufServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 0})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
