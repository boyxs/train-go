package redislock

import (
	"context"
	"fmt"
)

// acquireFencing 带 fencing 的单次获取：全新获取时在同一 Lua 内原子 INCR 单调计数器，
// 令牌随句柄返回（Fence()）；重入不 bump（沿用首次令牌）；被占返回 (nil,false,nil)。
//
// 为什么需要 fencing（唯一真安全）：持有者 GC/STW 暂停 → 锁 TTL 过期被别人拿走 →
// 暂停结束仍以为持锁去写 → 双写。锁本身挡不住这个时序，唯一解 = 单调令牌 + 资源侧校验。
func (c *RedisClient) acquireFencing(ctx context.Context, key, token string, cfg *lockConfig) (RedisLock, bool, error) {
	raw, err := fenceScript.Run(ctx, c.cmd, []string{lockKey(key), fenceKey(key)}, cfg.leaseMs(), token).Slice()
	if err != nil {
		return nil, false, err
	}
	if len(raw) != 2 {
		return nil, false, fmt.Errorf("redislock: 非法 fencing 获取结果 %v", raw)
	}
	status, _ := raw[0].(int64)
	fence, _ := raw[1].(int64)
	if status != -1 {
		return nil, false, nil // 被占，status 为剩余 pttl
	}
	return newLock(c, key, token, fence, cfg), true, nil
}

// FenceAccepted 资源侧 fencing 校验（应用层写法，§3.3 "二选一"之一）：
// 写被保护资源前，把本次 Fence() 令牌与资源已存的最大令牌比对，只有严格更大才放行；
// 更小 / 相等说明有更新的持有者已写过（自己多半是 GC 暂停后的过期持有者），必须拒绝。
//
//	incoming := lk.Fence()
//	if !redislock.FenceAccepted(incoming, stored.LastFence) { return ErrStaleFence }
//	// 放行后落库时把 incoming 持久化为新的 LastFence
//
// 另一种等价写法（DB 条件写，把校验合并进 SQL，天然原子、免读-改-写竞态）：
//
//	UPDATE res SET data = ?, fence = ? WHERE id = ? AND fence < ?  -- 影响行数=0 → 拒绝
//
// 未接任一资源侧校验 = 没上安全锁，只是"大概率互斥"（cron 幂等重算属可接受的 best-effort）。
func FenceAccepted(incoming, stored int64) bool {
	return incoming > stored
}
