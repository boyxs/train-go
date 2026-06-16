package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	userv1 "webook/sandbox/grpc/gen/user/v1"
)

const bufSize = 1024 * 1024

type UserServerSuite struct {
	suite.Suite
}

// startBufServer 起一个内存 gRPC server（bufconn），返回已连上的 client。
// 通过 t.Cleanup 自动注销。
func (s *UserServerSuite) startBufServer() userv1.UserServiceClient {
	t := s.T()
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
	s.Require().NoError(err)

	t.Cleanup(func() {
		s.Require().NoError(conn.Close())
		srv.GracefulStop()
	})

	return userv1.NewUserServiceClient(conn)
}

// startTCPServer 用真实 TCP 端口起 server，演示 bufconn 之外的连接方式。
// 端口取 0 由系统自动分配，避免和其他测试或残留进程抢固定端口。
func (s *UserServerSuite) startTCPServer() userv1.UserServiceClient {
	t := s.T()
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)

	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, NewMemoryUserServer())
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("grpc serve exit: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///"+lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	s.Require().NoError(err)

	t.Cleanup(func() {
		s.Require().NoError(conn.Close())
		srv.GracefulStop()
	})

	return userv1.NewUserServiceClient(conn)
}

// TestGetUser_OverTCP 走真实 TCP 端口验证同一套实现。
func (s *UserServerSuite) TestGetUser_OverTCP() {
	client := s.startTCPServer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	s.Require().NoError(err)
	s.Equal(int64(1), u.GetId())
	s.Equal("Alice", u.GetName())
}

func (s *UserServerSuite) TestGetUser_Found_EmailContact() {
	client := s.startBufServer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 1})
	s.Require().NoError(err)
	s.Require().NotNil(u)

	s.Equal(int64(1), u.GetId())
	s.Equal("Alice", u.GetName())
	s.Equal("https://cdn.example.com/a.png", u.GetAvatar())
	s.Equal(int32(28), u.GetAge())
	s.Equal("admin", u.GetAttributes()["role"])
	s.ElementsMatch([]string{"Ally", "Lis"}, u.GetNicknames())
	s.Equal("Shenzhen", u.GetAddress().GetCity())
	s.Equal(userv1.Gender_GENDER_FEMALE, u.GetGender())
	s.Equal("alice@example.com", u.GetEmail())
}

func (s *UserServerSuite) TestGetUser_Found_PhoneContact() {
	client := s.startBufServer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 2})
	s.Require().NoError(err)

	_, ok := u.GetContacts().(*userv1.User_Phone)
	s.True(ok, "expect oneof = Phone, got %T", u.GetContacts())
	s.Equal("13800000000", u.GetPhone())
}

func (s *UserServerSuite) TestGetUser_NotFound() {
	client := s.startBufServer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 999})
	s.Require().Error(err)
	s.Equal(codes.NotFound, status.Code(err))
}

func (s *UserServerSuite) TestGetUser_InvalidArgument() {
	client := s.startBufServer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: 0})
	s.Require().Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
}

func TestUserServer(t *testing.T) {
	suite.Run(t, new(UserServerSuite))
}
