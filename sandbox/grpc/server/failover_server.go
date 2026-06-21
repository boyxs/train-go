package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	userv1 "webook/sandbox/grpc/gen/user/v1"
)

// AlwaysFailedServer 模拟失败服务
type AlwaysFailedServer struct {
	userv1.UnimplementedUserServiceServer
	users map[int64]*userv1.User
	Flag  string
}

func NewAlwaysFailedServer(flag ...string) *AlwaysFailedServer {
	f := ""
	if len(flag) > 0 {
		f = flag[0]
	}
	return &AlwaysFailedServer{
		Flag: f,
		users: map[int64]*userv1.User{
			1: {
				Id:     1,
				Name:   "Alice",
				Avatar: "https://cdn.example.com/a.png",
				Attributes: map[string]string{
					"role": "admin",
				},
				Nicknames: []string{"Ally", "Lis"},
				Age:       proto.Int32(28),
				Address: &userv1.Address{
					Province: "Guangdong",
					City:     "Shenzhen",
				},
				Contacts: &userv1.User_Email{Email: "alice@example.com"},
				Gender:   userv1.Gender_GENDER_FEMALE,
			},
			2: {
				Id:       2,
				Name:     "Bob",
				Contacts: &userv1.User_Phone{Phone: "13800000000"},
				Gender:   userv1.Gender_GENDER_MALE,
			},
		},
	}
}

func (s *AlwaysFailedServer) GetUser(_ context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	fmt.Println("failover in")
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "id must be positive, got %d", req.GetId())
	}
	u, ok := s.users[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.GetId())
	}
	// 返回深拷贝：map 里存的是共享指针，直接返回会让并发请求拿到同一 message，
	// gRPC 随后对其做 marshal，而 proto message 并发 marshal 不保证安全。
	nu := proto.Clone(u).(*userv1.User)
	if s.Flag != "" {
		nu.Name = "failover from " + s.Flag
	}
	return nu, status.Errorf(codes.Unavailable, "模拟服务端异常")
}
