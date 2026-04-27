package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	userv1 "webook/sandbox/grpc/gen/user/v1"
)

// MemoryUserServer 内存版 UserService 实现，演示用。
type MemoryUserServer struct {
	userv1.UnimplementedUserServiceServer
	users map[int64]*userv1.User
}

func NewMemoryUserServer() *MemoryUserServer {
	return &MemoryUserServer{users: seedUsers()}
}

func (s *MemoryUserServer) GetUser(_ context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "id must be positive, got %d", req.GetId())
	}
	u, ok := s.users[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.GetId())
	}
	return u, nil
}

func seedUsers() map[int64]*userv1.User {
	return map[int64]*userv1.User{
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
	}
}
