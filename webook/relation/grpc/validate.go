package grpc

import (
	"context"

	"google.golang.org/grpc"

	relationv1 "github.com/boyxs/train-go/webook/api/gen/relation/v1"
	"github.com/boyxs/train-go/webook/relation/errs"
)

// ValidateUnaryInterceptor 入口统一校验写请求的 id 非空，handler 不再逐个写。
// 返回 *errs.Error，由外层 errconv 拦截器转 gRPC status。鉴权（uid 注入）与业务规则归网关 core / service。
func ValidateUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	switch r := req.(type) {
	case *relationv1.FollowRequest: // Follow / Unfollow 共用
		if r.GetFollowerId() <= 0 || r.GetFolloweeId() <= 0 {
			return nil, errs.ErrInvalidArg
		}
	case *relationv1.BlockRequest: // Block / Unblock 共用
		if r.GetUid() <= 0 || r.GetBlockedId() <= 0 {
			return nil, errs.ErrInvalidArg
		}
	}
	return handler(ctx, req)
}
