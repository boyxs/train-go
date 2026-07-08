-- 获取 / 重入：-1=成功（首次或同 token 重入）；>=0=被占（返回剩余 pttl 作等待提示）。
-- 存储统一 hash 模型 hash{ownerToken: 重入计数}，非重入即单 field 计数恒 1（§3.1 / ADR-3）。
-- KEYS[1]=redislock:{k}:lock  ARGV[1]=leaseMs  ARGV[2]=ownerToken
if redis.call('exists', KEYS[1]) == 0
        or redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
    redis.call('hincrby', KEYS[1], ARGV[2], 1) -- 首次=1 / 重入 +1
    redis.call('pexpire', KEYS[1], ARGV[1])
    return -1
end
local ttl = redis.call('pttl', KEYS[1])
if ttl < 0 then
    return tonumber(ARGV[1])
end
return ttl
