package grpc

import (
	"context"

	"google.golang.org/grpc"

	interactionv1 "github.com/webook/api/gen/interaction/v1"
	"github.com/webook/interaction/errs"
)

// requireUserMethods 需登录（uid>0）的方法集。
// GetInteraction 虽带 uid 但允许匿名（uid=0 时不回填 liked/collected），故不在内。
var requireUserMethods = map[string]struct{}{
	interactionv1.InteractionService_Like_FullMethodName:               {},
	interactionv1.InteractionService_CancelLike_FullMethodName:         {},
	interactionv1.InteractionService_Collect_FullMethodName:            {},
	interactionv1.InteractionService_CancelCollect_FullMethodName:      {},
	interactionv1.InteractionService_GetUserState_FullMethodName:       {},
	interactionv1.InteractionService_GetUserLiked_FullMethodName:       {},
	interactionv1.InteractionService_GetCollectedBizIds_FullMethodName: {},
}

// ValidateUnaryInterceptor 入口统一校验互动请求公共字段，handler 不再逐个写：
//   - 需登录方法（requireUserMethods）：uid > 0
//   - 所有请求：biz 非空
//   - 单实体请求（实现 GetBizId）：biz_id > 0（批量请求是 GetBizIds，自动跳过）
//
// 返回 *errs.Error，由外层 errconv 拦截器统一转 gRPC status。biz 白名单等业务规则仍归网关 core。
func ValidateUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if _, need := requireUserMethods[info.FullMethod]; need {
		if r, ok := req.(interface{ GetUid() int64 }); ok && r.GetUid() <= 0 {
			return nil, errs.ErrUnauthenticated
		}
	}
	if r, ok := req.(interface{ GetBiz() string }); ok && r.GetBiz() == "" {
		return nil, errs.ErrBizEmpty
	}
	if r, ok := req.(interface{ GetBizId() int64 }); ok && r.GetBizId() <= 0 {
		return nil, errs.ErrBizIdEmpty
	}
	return handler(ctx, req)
}
