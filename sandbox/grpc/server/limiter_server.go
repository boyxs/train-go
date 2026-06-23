package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	userv1 "webook/sandbox/grpc/gen/user/v1"
)

// LimiterServer 限流服务
type LimiterServer struct {
	userv1.UnimplementedUserServiceServer
	users map[int64]*userv1.User
	Flag  string
}

func NewLimiterServer(flag ...string) *LimiterServer {
	f := ""
	if len(flag) > 0 {
		f = flag[0]
	}
	return &LimiterServer{
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

func (s *LimiterServer) GetUser(_ context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	// 模拟处理耗时:放行的请求停在 handler 里,把在途数稳定压住阈值,
	// 后到的并发请求才会持续被限流——否则 handler 太快,瞬时在途数难超阈值。
	time.Sleep(500 * time.Millisecond)
	fmt.Println("limiter in")
	//limited := true
	//if limited {
	//	return nil, status.Errorf(codes.ResourceExhausted, "触发限流")
	//}
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "id must be positive, got %d", req.GetId())
	}
	u, ok := s.users[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.GetId())
	}
	nu := proto.Clone(u).(*userv1.User)
	if s.Flag != "" {
		nu.Name = "failover from " + s.Flag
	}
	return nu, nil
}
