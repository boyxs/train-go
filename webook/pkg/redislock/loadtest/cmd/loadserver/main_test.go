package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/redislock"
)

// 用真实并发 HTTP（模拟 JMeter 线程：acquire→持有→release）自测服务端不变量跟踪逻辑。
// JMeter 本身由使用者本机跑，这里验证 /acquire、/release、/stats 三件套在并发下互斥/fence 不破。
func TestLoadServer_ConcurrentAcquireRelease(t *testing.T) {
	rdb, backend := testRedis(t)
	t.Logf("backend=%s", backend)

	s := newServer(redislock.NewClient(rdb))
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	client := &http.Client{Timeout: 5 * time.Second}

	const (
		workers  = 16
		keyCount = 2 // 强竞争
		dur      = 1500 * time.Millisecond
	)
	deadline := time.Now().Add(dur)
	var wg sync.WaitGroup
	var attempts int64
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				key := fmt.Sprintf("jmeter:key:%d", id%keyCount)
				atomic.AddInt64(&attempts, 1)
				token := doAcquire(t, client, ts.URL, key)
				if token == "" {
					continue // busy
				}
				time.Sleep(2 * time.Millisecond) // think-time（模拟临界区）
				doRelease(t, client, ts.URL, key, token)
			}
		}(w)
	}
	wg.Wait()

	st := getStats(t, client, ts.URL)
	t.Logf("stats=%+v attempts=%d", st, attempts)
	assert.Zero(t, st["mutexViolations"], "互斥被破坏")
	assert.Zero(t, st["fenceMonotonicBreaks"], "fencing 令牌非单调")
	assert.Positive(t, st["acquired"], "应至少获取若干次")
	assert.Zero(t, st["activeHolds"], "结束后不应有残留持有")
}

// watchdog 参数端到端：lease=0s + watchdog=200ms，持有 600ms（远超租约）后另一方仍抢不到，
// 证明 watchdog 在持有期反复续约保活。仅真 Redis 有意义（miniredis 虚拟时钟不按 wall-clock 过期）。
func TestLoadServer_WatchdogParam_KeepsAlive(t *testing.T) {
	rdb, backend := testRedis(t)
	if backend == "miniredis" {
		t.Skip("watchdog 保活需真 Redis 的 wall-clock 过期，miniredis 不适用")
	}
	s := newServer(redislock.NewClient(rdb))
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	c := &http.Client{Timeout: 5 * time.Second}
	const key = "jmeter:wd"

	token := doAcquireWD(t, c, ts.URL, key)
	require.NotEmpty(t, token, "首次应抢到")

	time.Sleep(600 * time.Millisecond) // 远超 200ms watchdog 租约

	other := doAcquireWD(t, c, ts.URL, key) // 续约保活则 busy；锁过期则抢到（互斥被破坏）
	if other != "" {
		doRelease(t, c, ts.URL, key, other)
	}
	assert.Empty(t, other, "watchdog 应续约保活，另一方应抢不到")

	doRelease(t, c, ts.URL, key, token)
	assert.Zero(t, getStats(t, c, ts.URL)["mutexViolations"])
}

// 公平锁参数端到端：多线程 fair=true&wait=2s 抢同一 key，排队 FIFO 获取，服务端互斥不变量不破。
func TestLoadServer_FairParam(t *testing.T) {
	rdb, backend := testRedis(t)
	t.Logf("backend=%s", backend)
	s := newServer(redislock.NewClient(rdb))
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	c := &http.Client{Timeout: 5 * time.Second}

	const (
		workers = 8
		key     = "jmeter:fair"
		dur     = 1500 * time.Millisecond
	)
	deadline := time.Now().Add(dur)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				token := doAcquireFair(t, c, ts.URL, key)
				if token == "" {
					continue
				}
				time.Sleep(2 * time.Millisecond) // 临界区 think-time
				doRelease(t, c, ts.URL, key, token)
			}
		}()
	}
	wg.Wait()

	st := getStats(t, c, ts.URL)
	t.Logf("fair stats=%+v", st)
	assert.Zero(t, st["mutexViolations"], "公平锁互斥被破坏")
	assert.Positive(t, st["acquired"], "应有成功获取")
	assert.Zero(t, st["activeHolds"], "结束后不应有残留持有")
}

// /reset 清零计数 + 释放残留句柄，回到干净起点（多轮压测之间免重启）。
func TestLoadServer_Reset(t *testing.T) {
	rdb, _ := testRedis(t)
	s := newServer(redislock.NewClient(rdb))
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	c := &http.Client{Timeout: 5 * time.Second}

	// 制造计数 + 一个残留持有（acquire 不 release，模拟 shutdown 边界的孤儿）
	token := doAcquire(t, c, ts.URL, "jmeter:reset")
	require.NotEmpty(t, token)
	st := getStats(t, c, ts.URL)
	require.Positive(t, st["acquired"])
	require.Equal(t, int64(1), st["activeHolds"], "有一个未释放的持有")

	resp, err := c.Post(ts.URL+"/reset", "", nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	st = getStats(t, c, ts.URL)
	assert.Zero(t, st["acquired"], "reset 后计数应归零")
	assert.Zero(t, st["activeHolds"], "reset 应释放残留句柄")
	assert.Zero(t, st["mutexViolations"])
}

// acquireQ POST /acquire?<query>，返回 token（409 busy 返 ""）。三个 do* 包装它传各自参数。
func acquireQ(t *testing.T, c *http.Client, base, query string) string {
	t.Helper()
	resp, err := c.Post(base+"/acquire?"+query, "", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusConflict {
		return ""
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var b struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&b))
	return b.Token
}

func doAcquireWD(t *testing.T, c *http.Client, base, key string) string {
	return acquireQ(t, c, base, "key="+key+"&lease=0s&watchdog=200ms")
}

func doAcquire(t *testing.T, c *http.Client, base, key string) string {
	return acquireQ(t, c, base, "key="+key+"&lease=3s&fencing=true")
}

func doAcquireFair(t *testing.T, c *http.Client, base, key string) string {
	return acquireQ(t, c, base, "key="+key+"&lease=3s&fair=true&wait=2s")
}

func doRelease(t *testing.T, c *http.Client, base, key, token string) {
	t.Helper()
	resp, err := c.Post(base+"/release?key="+key+"&token="+token, "", nil)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func getStats(t *testing.T, c *http.Client, base string) map[string]int64 {
	t.Helper()
	resp, err := c.Get(base + "/stats")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	m := map[string]int64{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	return m
}

func testRedis(t *testing.T) (redis.UniversalClient, string) {
	addr := os.Getenv("REDISLOCK_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDISLOCK_REDIS_PASS")})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err == nil {
		t.Cleanup(func() { _ = rdb.Close() })
		return rdb, "real(" + addr + ")"
	}
	_ = rdb.Close()
	mr := miniredis.RunT(t)
	m := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = m.Close() })
	return m, "miniredis"
}
