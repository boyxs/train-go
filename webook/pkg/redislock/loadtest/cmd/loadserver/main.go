// Command loadserver 把 redislock 的获取/释放暴露成 HTTP 端点，供 JMeter 等协议压测工具驱动。
// JMeter 打 /acquire 提取 token → think-time → /release；互斥/fencing 不变量由本服务在服务端
// 用共享计数器跟踪（客户端侧看不到），/stats 暴露、/metrics 出 webook_lock_* 供 Grafana。
//
//	go run ./pkg/redislock/loadtest/cmd/loadserver -addr :8099   (Redis 走 env REDISLOCK_REDIS_ADDR/PASS)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/pkg/redislock"
	lockprom "github.com/boyxs/train-go/webook/pkg/redislock/prometheus"
)

// keyState 每 key 的不变量状态：holders=当前持有者数（互斥），lastFence=已见最大令牌（单调）。
type keyState struct {
	holders   int32
	lastFence int64
}

type server struct {
	cli     redislock.Client
	handles sync.Map // token(string) -> redislock.RedisLock（跨 /acquire、/release 两次请求持有句柄）
	keys    sync.Map // key(string)   -> *keyState

	acquired, busy, errs, released, releaseErr, mutexViol, fenceViol, active int64
}

func newServer(cli redislock.Client) *server { return &server{cli: cli} }

func (s *server) keyState(key string) *keyState {
	v, _ := s.keys.LoadOrStore(key, &keyState{})
	return v.(*keyState)
}

// acquireOpts 从 query 解析 Options：
//
//	lease    固定租约，>0 关 watchdog；=0s 走 watchdog（缺省 3s）
//	watchdog lease=0s 时自定义 watchdog 超时（续约每 /3）；缺省用库默认 30s
//	wait     TryLock 等待上限；fencing=true 启用 fencing
func acquireOpts(q map[string][]string) ([]redislock.Options, error) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	var opts []redislock.Options
	leaseStr := get("lease")
	if leaseStr == "" {
		leaseStr = "3s"
	}
	lease, err := time.ParseDuration(leaseStr)
	if err != nil {
		return nil, err
	}
	if lease > 0 {
		opts = append(opts, redislock.WithLeaseTime(lease)) // 固定租约、关 watchdog
	} else if wd := get("watchdog"); wd != "" { // lease=0s → watchdog，可自定义超时
		d, err := time.ParseDuration(wd)
		if err != nil {
			return nil, err
		}
		opts = append(opts, redislock.WithWatchdogTimeout(d))
	}
	if w := get("wait"); w != "" {
		d, err := time.ParseDuration(w)
		if err != nil {
			return nil, err
		}
		opts = append(opts, redislock.WithWaitTime(d))
	}
	if get("fencing") == "true" {
		opts = append(opts, redislock.WithFencing())
	}
	return opts, nil
}

func (s *server) handleAcquire(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"missing key"}`, http.StatusBadRequest)
		return
	}
	opts, err := acquireOpts(r.URL.Query())
	if err != nil {
		http.Error(w, `{"error":"bad opts"}`, http.StatusBadRequest)
		return
	}
	lk, ok, err := s.cli.TryLock(r.Context(), key, opts...)
	if err != nil {
		atomic.AddInt64(&s.errs, 1)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		atomic.AddInt64(&s.busy, 1)
		writeJSON(w, http.StatusConflict, map[string]any{"busy": true})
		return
	}
	token := lk.Token()
	s.handles.Store(token, lk)
	atomic.AddInt64(&s.active, 1)

	ks := s.keyState(key)
	if atomic.AddInt32(&ks.holders, 1) > 1 { // 同 key 出现第二个持有者 → 互斥被破坏
		atomic.AddInt64(&s.mutexViol, 1)
	}
	fence := lk.Fence()
	if fence > 0 {
		if fence <= atomic.LoadInt64(&ks.lastFence) {
			atomic.AddInt64(&s.fenceViol, 1)
		} else {
			atomic.StoreInt64(&ks.lastFence, fence)
		}
	}
	atomic.AddInt64(&s.acquired, 1)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "fence": fence})
}

func (s *server) handleRelease(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	token := r.URL.Query().Get("token")
	v, loaded := s.handles.LoadAndDelete(token)
	if !loaded { // 未知/已释放 token（JMeter 未持有却来释放，或重复释放）
		writeJSON(w, http.StatusGone, map[string]any{"released": false})
		return
	}
	atomic.AddInt64(&s.active, -1)
	// 先退出临界区计数、再真释放：避免"减到 0 后、Redis 锁尚在时"被他人抢到造成假阳性
	atomic.AddInt32(&s.keyState(key).holders, -1)

	lk := v.(redislock.RedisLock)
	if err := lk.Unlock(r.Context()); err != nil {
		atomic.AddInt64(&s.releaseErr, 1)
	} else {
		atomic.AddInt64(&s.released, 1)
	}
	writeJSON(w, http.StatusOK, map[string]any{"released": true})
}

func (s *server) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"acquired":             atomic.LoadInt64(&s.acquired),
		"busy":                 atomic.LoadInt64(&s.busy),
		"errors":               atomic.LoadInt64(&s.errs),
		"released":             atomic.LoadInt64(&s.released),
		"releaseErrors":        atomic.LoadInt64(&s.releaseErr),
		"activeHolds":          atomic.LoadInt64(&s.active),
		"mutexViolations":      atomic.LoadInt64(&s.mutexViol), // ★ 必须 0
		"fenceMonotonicBreaks": atomic.LoadInt64(&s.fenceViol), // ★ 必须 0
	})
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/acquire", s.handleAcquire)
	mux.HandleFunc("/release", s.handleRelease)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func main() {
	addr := flag.String("addr", ":8099", "HTTP 监听地址")
	flag.Parse()

	redisAddr := os.Getenv("REDISLOCK_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("REDISLOCK_REDIS_PASS")})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis 不可达 %s: %v", redisAddr, err)
	}

	// 包 prometheus：/metrics 出 webook_lock_*（acquire/held/wait/watchdog_lost/fence_issued）
	reg := prometheus.NewRegistry()
	cli := lockprom.NewPrometheusBuilder("webook", "lock", "loadserver").Registry(reg).Build(redislock.NewClient(rdb))

	s := newServer(cli)
	mux := s.routes()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	log.Printf("redislock loadserver 监听 %s，Redis=%s", *addr, redisAddr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
