package grpc

import (
	"context"

	"google.golang.org/grpc"

	feedv1 "github.com/boyxs/train-go/webook/api/gen/feed/v1"
	"github.com/boyxs/train-go/webook/feed/errs"
)

// ValidateUnaryInterceptor 入口统一校验请求 id 非空，handler 不再逐个写。
// 返回 *errs.Error，由外层 errconv 拦截器转 gRPC status。鉴权（uid 注入）归网关 core。
func ValidateUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	switch r := req.(type) {
	case *feedv1.ListFeedRequest:
		if r.GetUid() <= 0 {
			return nil, errs.ErrInvalidArg
		}
	case *feedv1.NewCountRequest:
		if r.GetUid() <= 0 {
			return nil, errs.ErrInvalidArg
		}
	case *feedv1.FanoutArticleRequest:
		if r.GetArticleId() <= 0 || r.GetAuthorId() <= 0 {
			return nil, errs.ErrInvalidArg
		}
	case *feedv1.RemoveArticleRequest:
		if r.GetArticleId() <= 0 || r.GetAuthorId() <= 0 {
			return nil, errs.ErrInvalidArg
		}
	case *feedv1.InvalidateInboxesRequest:
		if len(r.GetUids()) == 0 {
			return nil, errs.ErrInvalidArg
		}
	}
	return handler(ctx, req)
}
