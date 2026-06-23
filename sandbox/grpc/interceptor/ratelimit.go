package interceptor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ecodeclub/ekit/queue"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Limiter interface {
	Build() grpc.UnaryServerInterceptor
}
type Closer interface {
	Close()
}

// CounterLimiter 并发计数器限流
type CounterLimiter struct {
	cnt       *atomic.Int32
	threshold int32
}

func NewCounterLimiter() Limiter {
	return &CounterLimiter{
		cnt:       &atomic.Int32{},
		threshold: int32(10),
	}
}

func (l *CounterLimiter) Build() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		cnt := l.cnt.Add(1)
		defer func() {
			l.cnt.Add(-1)
		}()
		if cnt > l.threshold {
			return nil, status.Errorf(codes.ResourceExhausted, "触发限流")
		}
		return handler(ctx, req)
	}
}

// FixedWindowLimiter 固定窗口算法限流
type FixedWindowLimiter struct {
	window      time.Duration
	windowStart time.Time
	cnt         int
	threshold   int
	lock        sync.Mutex
}

func NewFixedWindowLimiter() Limiter {
	return &FixedWindowLimiter{
		window:      time.Minute,
		windowStart: time.Now(),
		cnt:         0,
		threshold:   20,
	}
}

func (l *FixedWindowLimiter) Build() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		l.lock.Lock()
		now := time.Now()
		// 切换窗口
		if now.After(l.windowStart.Add(l.window)) {
			l.windowStart = now
			l.cnt = 0
		}
		l.cnt++
		if l.cnt <= l.threshold {
			l.lock.Unlock()
			return handler(ctx, req)
		}
		l.lock.Unlock()
		return nil, status.Errorf(codes.ResourceExhausted, "触发限流")
	}
}

// SlidingWindowLimiter 滑动窗口算法限流
type SlidingWindowLimiter struct {
	window    time.Duration
	pq        *queue.PriorityQueue[time.Time]
	threshold int
	lock      sync.Mutex
}

func NewSlidingWindowLimiter() Limiter {
	return &SlidingWindowLimiter{
		window: 10 * time.Second,
		pq: queue.NewPriorityQueue(30, func(a, b time.Time) int {
			// 返回 -1 表示 a 优先级更高，即 a 最早且最先出队
			return a.Compare(b)
		}),
		threshold: 30,
	}
}

func (l *SlidingWindowLimiter) Build() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		l.lock.Lock()
		now := time.Now()
		windowStart := now.Add(-l.window)
		for l.pq.Len() > 0 {
			earliest, err := l.pq.Peek()
			if err != nil {
				break
			}
			// 最早的都在窗口内,停止清理
			if earliest.After(windowStart) {
				break
			}
			_, _ = l.pq.Dequeue()
		}
		if l.pq.Len() < l.threshold {
			_ = l.pq.Enqueue(now)
			l.lock.Unlock()
			return handler(ctx, req)
		}
		l.lock.Unlock()
		return nil, status.Errorf(codes.ResourceExhausted, "触发限流")
	}
}

// TokenBucketLimiter 令牌桶算法限流
type TokenBucketLimiter struct {
	capacity  int
	tokens    int
	ticker    *time.Ticker
	closeCh   chan struct{}
	closeOnce sync.Once
	lock      sync.Mutex
}

// NewTokenBucketLimiter
//
//	interval: 每隔多久加一个令牌,如 100ms 加一个 = 10/s
//	capacity: 桶容量(突发上限)
func NewTokenBucketLimiter() Limiter {
	// capacity & interval 可为参数传递进来
	capacity := 40
	interval := 100 * time.Millisecond
	return &TokenBucketLimiter{
		capacity: capacity,
		tokens:   capacity,
		ticker:   time.NewTicker(interval),
		closeCh:  make(chan struct{}),
	}
}

func (l *TokenBucketLimiter) Build() grpc.UnaryServerInterceptor {
	// 补令牌协程随 Build 启动一次(放进下面每请求执行的闭包会逐请求泄漏),Close 后退出
	go func() {
		for {
			select {
			case <-l.ticker.C:
				l.lock.Lock()
				if l.tokens < l.capacity {
					// 加一个,但不超过容量
					l.tokens++
				}
				l.lock.Unlock()
			case <-l.closeCh:
				return
			}
		}
	}()
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		l.lock.Lock()
		if l.tokens > 0 {
			l.tokens--
			l.lock.Unlock()
			return handler(ctx, req)
		}
		l.lock.Unlock()
		return nil, status.Errorf(codes.ResourceExhausted, "触发限流")
	}
}

func (l *TokenBucketLimiter) Close() {
	l.closeOnce.Do(func() {
		l.ticker.Stop()
		close(l.closeCh)
	})
}

// RateTokenBucketLimiter 生产级 令牌桶算法限流
type RateTokenBucketLimiter struct {
	limiter *rate.Limiter
}

// NewRateTokenBucketLimiter
//
//	r:     稳态速率(每秒补充)
//	burst: 桶容量(瞬时能放行的突发量)
func NewRateTokenBucketLimiter() Limiter {
	// r & burst 可为参数传递进来
	r := rate.Limit(10)
	burst := 40
	return &RateTokenBucketLimiter{
		limiter: rate.NewLimiter(r, burst),
	}
}

func (l *RateTokenBucketLimiter) Build() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if l.limiter.Allow() {
			return handler(ctx, req)
		}
		return nil, status.Errorf(codes.ResourceExhausted, "触发限流")
	}
}

// LeakyBucketLimiter 漏桶算法限流
type LeakyBucketLimiter struct {
	ticker    *time.Ticker
	queue     chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
}

// NewLeakyBucketLimiter
//
//	interval: 漏出间隔,如每 100ms 放一个 = 10/s
//	maxQueue: 最大排队数,超过立即拒绝
func NewLeakyBucketLimiter() Limiter {
	// interval & maxQueue 可为参数传递进来
	interval := 100 * time.Millisecond
	maxQueue := 10
	return &LeakyBucketLimiter{
		ticker:  time.NewTicker(interval),
		queue:   make(chan struct{}, maxQueue),
		closeCh: make(chan struct{}),
	}
}

func (l *LeakyBucketLimiter) Build() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		// 抢排队名额,满了直接拒绝(不阻塞)
		select {
		case l.queue <- struct{}{}:
			defer func() {
				<-l.queue
			}()
		default:
			return nil, status.Errorf(codes.ResourceExhausted, "队列已满,触发限流")
		}
		// 排队等漏出
		select {
		case <-l.ticker.C:
			return handler(ctx, req)
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		case <-l.closeCh:
			return nil, status.Errorf(codes.Unavailable, "限流器已关闭")
		}
	}
}

func (l *LeakyBucketLimiter) Close() {
	l.closeOnce.Do(func() {
		l.ticker.Stop()
		close(l.closeCh)
	})
}
