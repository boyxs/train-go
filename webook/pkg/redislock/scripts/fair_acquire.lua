-- 公平获取（FIFO 排队 + 死等待者逐出），全程原子。
-- 返回：-1=获取/重入成功；>=0=未获取（锁剩余 pttl，作等待提示）。
-- KEYS[1]=lock(hash)  KEYS[2]=queue(list)  KEYS[3]=qts(zset)
-- ARGV[1]=leaseMs  ARGV[2]=ownerToken  ARGV[3]=nowMs  ARGV[4]=heartbeatMs

-- 1. 清理队头已超时（deadline<=now）的死等待者：崩溃/放弃者不再堵死队列
while true do
    local head = redis.call('lindex', KEYS[2], 0)
    if not head then break end
    local dl = redis.call('zscore', KEYS[3], head)
    if (not dl) or (tonumber(dl) > tonumber(ARGV[3])) then break end
    redis.call('lpop', KEYS[2])
    redis.call('zrem', KEYS[3], head)
end

-- 2. 重入：已被我持有 → 计数 +1（公平锁同样支持重入）
if redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
    redis.call('hincrby', KEYS[1], ARGV[2], 1)
    redis.call('pexpire', KEYS[1], ARGV[1])
    return -1
end

-- 3. 锁空闲 且（队空 或 队头是我）→ 出队 + 获取
if redis.call('exists', KEYS[1]) == 0 then
    local head = redis.call('lindex', KEYS[2], 0)
    if (not head) or (head == ARGV[2]) then
        if head then redis.call('lpop', KEYS[2]) end
        redis.call('zrem', KEYS[3], ARGV[2])
        redis.call('hincrby', KEYS[1], ARGV[2], 1)
        redis.call('pexpire', KEYS[1], ARGV[1])
        return -1
    end
end

-- 4. 拿不到 → 首次入队尾 / 已在队则刷新 deadline（心跳），返回锁剩余 pttl
if not redis.call('zscore', KEYS[3], ARGV[2]) then
    redis.call('rpush', KEYS[2], ARGV[2])
end
redis.call('zadd', KEYS[3], tonumber(ARGV[3]) + tonumber(ARGV[4]), ARGV[2])
local ttl = redis.call('pttl', KEYS[1])
if ttl < 0 then
    ttl = 0
end
return ttl
