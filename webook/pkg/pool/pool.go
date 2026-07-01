// Package pool 提供固定 worker 数 + 有界队列的通用协程池，设计参考 Java ThreadPoolExecutor：
// 可配拒绝策略（Discard / CallerRuns / Block）、优雅关闭 vs 立即关闭、panic 兜底、运行态可观测。
//
// 两个关键设计：
//   - task channel 永不 close（关闭走独立 done channel）→ 关闭后 Submit 安全返 false，绝不 panic（send on closed channel）。
//   - 单队列而非分片：分片队列实测反而回归（选片争用 + 牺牲「空闲 worker 抢下一个任务」的负载均衡），
//     单 channel 8 核 ~7M ops/s 已远超本项目所需。证伪过程见 pool_bench_test.go。
//
// 为何不复用 golang.org/x/sync：semaphore / errgroup 只能「阻塞或报错」，没有队列满即丢（Discard）策略，
// 也没有队列深度 / 在途任务的可观测指标——而 ranking boost 的「全量重算兜底」恰恰要 Discard + drop 计数，
// 故自建。注入与 Prometheus 指标见 internal/ioc/interaction.go。
package pool

import (
	"sync"
	"sync/atomic"
	"time"
)

// RejectPolicy 队列满时的拒绝策略
type RejectPolicy int

const (
	// Discard 丢弃新任务，Submit 返回 false。默认。
	Discard RejectPolicy = iota
	// CallerRuns 由调用方 goroutine 直接执行，形成背压。
	CallerRuns
	// Block 阻塞等待空位，直到入队成功 / 超时（WithBlockTimeout）/ 池关闭。
	Block
)

// Pool 有界协程池。
type Pool struct {
	tasks     chan func()   // 任务队列；永不 close（关闭安全见包注释）
	done      chan struct{} // 关闭信号，替代 close(tasks)
	wg        sync.WaitGroup
	closeOnce sync.Once
	closed    atomic.Bool
	drain     bool // 关闭时是否排空队列；Shutdown=true / ShutdownNow=false（closeOnce 内单次写）

	policy       RejectPolicy
	blockTimeout time.Duration // Block 策略超时，<=0 表示无限等
	onPanic      func(any)
	inFlight     atomic.Int64 // 在途（in-flight）任务数，供 InFlight()
}

// Option 函数式配置。
type Option func(*Pool)

// WithRejectPolicy 设置队列满时的拒绝策略，默认 Discard。
func WithRejectPolicy(p RejectPolicy) Option { return func(pl *Pool) { pl.policy = p } }

// WithBlockTimeout 设置 Block 策略的最长等待，<=0 为无限等。
func WithBlockTimeout(d time.Duration) Option { return func(pl *Pool) { pl.blockTimeout = d } }

// WithPanicHandler 设置单个 task panic 的回调（如记日志）；不设则静默兜底。
func WithPanicHandler(fn func(any)) Option { return func(pl *Pool) { pl.onPanic = fn } }

// New 启动 workers 个常驻 worker，队列容量 queueSize。
func New(workers, queueSize int, opts ...Option) *Pool {
	if workers <= 0 {
		workers = 1
	}
	if queueSize < 0 {
		queueSize = 0
	}
	p := &Pool{
		tasks:  make(chan func(), queueSize),
		done:   make(chan struct{}),
		policy: Discard,
	}
	for _, o := range opts {
		if o != nil {
			o(p)
		}
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		select { // 关闭优先：ShutdownNow 后不再拾取新的排队任务
		case <-p.done:
			if p.drain {
				p.drainQueue()
			}
			return
		default:
		}
		select {
		case task := <-p.tasks:
			p.run(task)
		case <-p.done:
			if p.drain { // 优雅关闭：排空已入队任务再退出
				p.drainQueue()
			}
			return
		}
	}
}

func (p *Pool) drainQueue() {
	for {
		select {
		case task := <-p.tasks:
			p.run(task)
		default:
			return
		}
	}
}

// run 包一层 recover：单个 task panic 不拖垮 worker（否则池容量会永久缩水）。
func (p *Pool) run(task func()) {
	p.inFlight.Add(1)
	defer p.inFlight.Add(-1)
	defer func() {
		if r := recover(); r != nil && p.onPanic != nil {
			p.onPanic(r)
		}
	}()
	task()
}

// Submit 提交任务。true=已接受（入队 / CallerRuns 已执行），false=被拒绝（队列满走 Discard、Block 超时、或池已关闭）。
func (p *Pool) Submit(task func()) bool {
	if task == nil || p.closed.Load() {
		return false
	}
	select { // 快路径：非阻塞入队
	case p.tasks <- task:
		return true
	default:
	}
	switch p.policy { // 队列满
	case CallerRuns:
		p.run(task)
		return true
	case Block:
		return p.submitBlocking(task)
	default: // Discard
		return false
	}
}

func (p *Pool) submitBlocking(task func()) bool {
	if p.blockTimeout <= 0 {
		select {
		case p.tasks <- task:
			return true
		case <-p.done:
			return false
		}
	}
	timer := time.NewTimer(p.blockTimeout)
	defer timer.Stop()
	select {
	case p.tasks <- task:
		return true
	case <-timer.C:
		return false
	case <-p.done:
		return false
	}
}

// Shutdown 优雅关闭：拒绝新任务，排空队列中已入队任务后，等所有 worker 退出。幂等。
func (p *Pool) Shutdown() {
	p.closed.Store(true)
	p.closeOnce.Do(func() {
		p.drain = true
		close(p.done)
	})
	p.wg.Wait()
}

// ShutdownNow 立即关闭：拒绝新任务，丢弃队列中未开始的任务（返回丢弃数），等在途任务跑完。幂等。
func (p *Pool) ShutdownNow() int {
	p.closed.Store(true)
	p.closeOnce.Do(func() {
		p.drain = false
		close(p.done)
	})
	p.wg.Wait()
	dropped := 0
	for {
		select {
		case <-p.tasks:
			dropped++
		default:
			return dropped
		}
	}
}

// InFlight 当前正在执行（in-flight）的任务数。
func (p *Pool) InFlight() int { return int(p.inFlight.Load()) }

// Queued 当前排队等待的任务数。
func (p *Pool) Queued() int { return len(p.tasks) }
