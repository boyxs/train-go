package loadtest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// TestLoad 真 Redis 并发压测（真 Redis 不可达则 skip）。逐模式校验互斥/单调不变量必须 = 0。
//
//	REDISLOCK_REDIS_PASS=xxx go test ./redislock/loadtest/ -run TestLoad -v
//	（地址走 env REDISLOCK_REDIS_ADDR，默认 127.0.0.1:6379）
func TestLoad(t *testing.T) {
	addr := os.Getenv("REDISLOCK_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDISLOCK_REDIS_PASS")})
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	pingErr := rdb.Ping(pingCtx).Err()
	cancel()
	if pingErr != nil {
		_ = rdb.Close()
		t.Skipf("跳过：真 Redis 不可达 %s: %v（设 REDISLOCK_REDIS_ADDR/PASS 开启）", addr, pingErr)
	}
	t.Cleanup(func() {
		keys, _ := rdb.Keys(context.Background(), "redislock:{loadtest:key:*").Result()
		if len(keys) > 0 {
			rdb.Del(context.Background(), keys...)
		}
		_ = rdb.Close()
	})
	cli := redislock.NewClient(rdb)

	cases := []struct {
		name string
		cfg  Config
	}{
		{"default", Config{Concurrency: 64, Duration: 3 * time.Second, KeyCount: 4, LeaseTime: 3 * time.Second, HoldTime: time.Millisecond}},
		{"fair", Config{Concurrency: 32, Duration: 3 * time.Second, KeyCount: 2, LeaseTime: 3 * time.Second, WaitTime: 5 * time.Second, Fair: true, HoldTime: time.Millisecond}},
		{"fencing", Config{Concurrency: 64, Duration: 3 * time.Second, KeyCount: 4, LeaseTime: 3 * time.Second, Fencing: true, HoldTime: time.Millisecond}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := tc.cfg.Run(context.Background(), cli)
			require.NoError(t, err)
			t.Logf("acquired=%d busy=%d errs=%d QPS=%.0f p50=%v p90=%v p99=%v mutexViol=%d fenceBreaks=%d",
				rep.Acquired, rep.Busy, rep.Errors, rep.QPS, rep.P50, rep.P90, rep.P99,
				rep.MutexViolations, rep.FenceMonotonicBreaks)
			assert.Zero(t, rep.MutexViolations, "互斥被破坏：同一 key 同时 >1 持有者")
			assert.Zero(t, rep.FenceMonotonicBreaks, "fencing 令牌非单调递增")
			assert.Positive(t, rep.Acquired, "应有成功获取")
		})
	}
}
