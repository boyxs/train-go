-- 带 fencing 的获取。返回数组 {status, fence}：
--   status: -1=成功（首次或重入）；>=0=被占（剩余 pttl 作等待提示）
--   fence : 全新获取返回新的单调令牌；重入 / 被占返回 0（重入沿用句柄缓存的首次令牌）
-- 单调计数器 KEYS[2] 只在全新获取时 INCR，且不设过期——持久保存，过期会导致单调断裂（§3.3 风险）。
-- KEYS[1]=redislock:{k}:lock  KEYS[2]=redislock:{k}:fence
-- ARGV[1]=leaseMs  ARGV[2]=ownerToken
if redis.call('exists', KEYS[1]) == 0 then
    redis.call('hincrby', KEYS[1], ARGV[2], 1)
    redis.call('pexpire', KEYS[1], ARGV[1])
    local f = redis.call('incr', KEYS[2]) -- 全新获取才 bump
    return { -1, f }
elseif redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
    redis.call('hincrby', KEYS[1], ARGV[2], 1)
    redis.call('pexpire', KEYS[1], ARGV[1])
    return { -1, 0 } -- 重入不 bump
end
local ttl = redis.call('pttl', KEYS[1])
if ttl < 0 then
    ttl = tonumber(ARGV[1])
end
return { ttl, 0 }