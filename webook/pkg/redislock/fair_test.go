package redislock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 公平锁：被占时阻塞的获取者应进入 FIFO 队列（queue），供后续按序发锁。
func TestFair_EnqueuesWaiters(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	bg := context.Background()
	key := "k1"

	holder, ok, err := cli.TryLock(bg, key, WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { _ = holder.Unlock(bg) })

	const n = 3
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lk, err := cli.Lock(ctx, key, WithFair(),
				WithLeaseTime(30*time.Second), WithRetryInterval(50*time.Millisecond))
			if err == nil {
				_ = lk.Unlock(bg)
			}
		}()
		// 等这个 waiter 真正入队，再起下一个 → 队列顺序确定
		want := int64(i + 1)
		require.Eventually(t, func() bool {
			return rdb.LLen(bg, queueKey(key)).Val() == want
		}, 2*time.Second, 10*time.Millisecond, "waiter %d 应进入公平队列", i)
	}

	assert.Equal(t, int64(n), rdb.LLen(bg, queueKey(key)).Val(), "3 个阻塞等待者应都在队列里")

	cancel()
	wg.Wait()
}

// 公平锁 FIFO：多个等待者按入队先后依次拿到锁（非抢占）。
func TestFair_FIFOOrder(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	bg := context.Background()
	key := "k1"

	holder, ok, err := cli.TryLock(bg, key, WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)

	const n = 4
	var mu sync.Mutex
	var order []int
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lk, err := cli.Lock(ctx, key, WithFair(),
				WithLeaseTime(30*time.Second), WithRetryInterval(50*time.Millisecond))
			if err != nil {
				return
			}
			mu.Lock()
			order = append(order, id)
			mu.Unlock()
			time.Sleep(20 * time.Millisecond) // 持有一会，让释放后下一队头成为唯一竞争者
			_ = lk.Unlock(bg)
		}(i)
		// 确保 id 入队早于 id+1（队列顺序 = 入队顺序）
		want := int64(i + 1)
		require.Eventually(t, func() bool {
			return rdb.LLen(bg, queueKey(key)).Val() == want
		}, 2*time.Second, 10*time.Millisecond, "waiter %d 应入队", i)
	}

	require.NoError(t, holder.Unlock(bg)) // 放锁，队头依次获取
	wg.Wait()

	assert.Equal(t, []int{0, 1, 2, 3}, order, "公平锁应按 FIFO 入队顺序发锁")
}

// 死等待者逐出：队头是个 deadline 已过的等待者（模拟崩溃/放弃未清理），
// 活获取者到来时应先清理它再拿锁，不被它永久堵死。
func TestFair_EvictsDeadWaiter(t *testing.T) {
	cli, _, rdb := newTestClient(t)
	bg := context.Background()
	key := "k1"

	// 注入队头死等待者：deadline 已是 1 分钟前
	dead := "dead-waiter"
	require.NoError(t, rdb.RPush(bg, queueKey(key), dead).Err())
	past := float64(time.Now().Add(-time.Minute).UnixMilli())
	require.NoError(t, rdb.ZAdd(bg, qtsKey(key), redis.Z{Score: past, Member: dead}).Err())

	// 锁空闲，活获取者来拿：fair_acquire 先逐出死队头，再因队空拿到
	lock, ok, err := cli.TryLock(bg, key, WithFair(), WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok, "死等待者应被逐出，活获取者应能拿到锁")
	t.Cleanup(func() { _ = lock.Unlock(bg) })

	assert.Equal(t, int64(0), rdb.LLen(bg, queueKey(key)).Val(), "死等待者应被逐出队列")
	assert.ErrorIs(t, rdb.ZScore(bg, qtsKey(key), dead).Err(), redis.Nil, "死等待者应从 qts 移除")
}

// 公平锁下重入：同 ownerId 连续获取即重入（走 fair_acquire step 2），释放 N 次才真释放。
func TestFair_Reentrant(t *testing.T) {
	cli, _, _ := newTestClient(t)
	bg := context.Background()
	key := "k1"

	l1, ok, err := cli.TryLock(bg, key, WithFair(), WithReentrant("owner-A"), WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	l2, ok, err := cli.TryLock(bg, key, WithFair(), WithReentrant("owner-A"), WithLeaseTime(30*time.Second))
	require.NoError(t, err)
	require.True(t, ok, "公平锁下同 owner 应重入成功")

	hc, err := l2.HoldCount(bg)
	require.NoError(t, err)
	assert.Equal(t, 2, hc, "重入深度应为 2")

	require.NoError(t, l1.Unlock(bg))
	locked, err := l2.IsLocked(bg)
	require.NoError(t, err)
	assert.True(t, locked, "释放一次仍应持有")
	require.NoError(t, l2.Unlock(bg))
}

// fair + fencing 暂不支持组合：入口 fail-loud，返回 ErrFairFencingUnsupported。
func TestFair_WithFencing_Unsupported(t *testing.T) {
	cli, _, _ := newTestClient(t)
	bg := context.Background()

	_, _, err := cli.TryLock(bg, "k1", WithFair(), WithFencing())
	assert.ErrorIs(t, err, ErrFairFencingUnsupported, "TryLock 组合 fair+fencing 应报错")

	_, err = cli.Lock(bg, "k1", WithFair(), WithFencing())
	assert.ErrorIs(t, err, ErrFairFencingUnsupported, "Lock 组合 fair+fencing 应报错")
}
