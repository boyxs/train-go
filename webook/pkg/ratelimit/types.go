package ratelimit

import "golang.org/x/net/context"

//go:generate mockgen -source=./types.go -package=limitmocks -destination=mocks/limiter.mock.go Limiter
type Limiter interface {
	Limit(ctx context.Context, key string) (bool, error)
}
