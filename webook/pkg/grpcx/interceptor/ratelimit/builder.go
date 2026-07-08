// Package ratelimit 提供 gRPC 限流拦截器：支持全局 / 服务级 / 方法级三档，
// 请求期按「最具体优先」(方法 > 服务 > 全局) 选中对应 limiter 判定。
package ratelimit

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/pkg/ratelimit"
)

const (
	keyPrefixSvc    = "ratelimit:service:"
	keyPrefixMethod = "ratelimit:method:"
)

type Builder interface {
	BuildUnaryServer() grpc.UnaryServerInterceptor
	BuildUnaryClient() grpc.UnaryClientInterceptor
}

// rule = limiter + 注册时预算好的 Redis 键，避免每请求拼接 key 产生分配。
type rule struct {
	lim ratelimit.Limiter
	key string
}

// InterceptorBuilder 是 Builder 的默认实现。
// limiter/key 为全局默认档（limiter 为 nil 时无全局限流）；
// services/methods 为按级别覆盖的规则，请求期「方法 > 服务 > 全局」优先。
type InterceptorBuilder struct {
	limiter     ratelimit.Limiter // 全局默认 limiter
	key         string            // 全局默认 key
	services    map[string]rule   // service FQN  → rule
	methods     map[string]rule   // full method → rule
	l           logger.LoggerX
	rejectOnErr bool // limiter 故障时：true 拒绝(保守，默认) / false 放行(激进)
}

// NewInterceptorBuilder 注入全局默认 limiter（可为 nil）与 key、LoggerX，
// 返回具体类型以支持链式 WithXxx。
func NewInterceptorBuilder(limiter ratelimit.Limiter, key string, l logger.LoggerX) *InterceptorBuilder {
	if l == nil {
		l = logger.NewNopLogger()
	}
	return &InterceptorBuilder{
		limiter:     limiter,
		key:         key,
		services:    make(map[string]rule),
		methods:     make(map[string]rule),
		l:           l,
		rejectOnErr: true,
	}
}

// WithService 为某 gRPC 服务（FQN，如 webook.search.v1.SearchService）注册限流。
func (b *InterceptorBuilder) WithService(service string, lim ratelimit.Limiter) *InterceptorBuilder {
	b.services[service] = rule{lim: lim, key: keyPrefixSvc + service}
	return b
}

// WithMethod 为某 full method（如 /webook.search.v1.SearchService/SearchArticles）注册限流。
func (b *InterceptorBuilder) WithMethod(fullMethod string, lim ratelimit.Limiter) *InterceptorBuilder {
	b.methods[fullMethod] = rule{lim: lim, key: keyPrefixMethod + fullMethod}
	return b
}

// WithMethods 批量为多个 full method 注册同一 limiter（各自独立桶）。
func (b *InterceptorBuilder) WithMethods(fullMethods []string, lim ratelimit.Limiter) *InterceptorBuilder {
	for _, m := range fullMethods {
		b.methods[m] = rule{lim: lim, key: keyPrefixMethod + m}
	}
	return b
}

// WithRejectOnErr 设置 limiter 故障时是否拒绝：true 拒绝(保守，默认) / false 放行(激进)。
func (b *InterceptorBuilder) WithRejectOnErr(rejectOnErr bool) *InterceptorBuilder {
	b.rejectOnErr = rejectOnErr
	return b
}

// serviceOf 从 full method 取服务名：/pkg.Service/Method → pkg.Service
func serviceOf(fullMethod string) string {
	svc, _, _ := strings.Cut(strings.TrimPrefix(fullMethod, "/"), "/")
	return svc
}

// resolve 按「最具体优先」选中 limiter 及其 Redis 键；无规则返回 (nil, "")。
func (b *InterceptorBuilder) resolve(fullMethod string) (ratelimit.Limiter, string) {
	if r, ok := b.methods[fullMethod]; ok {
		return r.lim, r.key
	}
	if svc := serviceOf(fullMethod); svc != "" {
		if r, ok := b.services[svc]; ok {
			return r.lim, r.key
		}
	}
	if b.limiter != nil {
		return b.limiter, b.key
	}
	return nil, ""
}

// allow 执行限流判定：无规则放行；被限/故障按 rejectOnErr 决定。server / client 共用。
func (b *InterceptorBuilder) allow(ctx context.Context, fullMethod string) (bool, error) {
	lim, key := b.resolve(fullMethod)
	if lim == nil {
		return true, nil
	}
	limited, err := lim.Limit(ctx, key)
	if err != nil {
		b.l.Error("限流器执行失败",
			logger.Error(err),
			logger.String("key", key),
			logger.String("method", fullMethod))
		if b.rejectOnErr {
			return false, status.Error(codes.Unavailable, "rate limiter unavailable")
		}
		return true, nil
	}
	if limited {
		return false, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}
	return true, nil
}

func (b *InterceptorBuilder) BuildUnaryServer() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		pass, err := b.allow(ctx, info.FullMethod)
		if !pass {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (b *InterceptorBuilder) BuildUnaryClient() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		pass, err := b.allow(ctx, method)
		if !pass {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
