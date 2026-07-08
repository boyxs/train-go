-- 释放：-1=不在我手里；0=重入未归零仍持有；1=完全释放并唤醒等待者。
-- KEYS[1]=redislock:{k}:lock  KEYS[2]=redislock:{k}:ch
-- ARGV[1]=leaseMs  ARGV[2]=ownerToken  ARGV[3]=unlockMsg
if redis.call('hexists', KEYS[1], ARGV[2]) == 0 then
    return -1
end
if redis.call('hincrby', KEYS[1], ARGV[2], -1) > 0 then
    redis.call('pexpire', KEYS[1], ARGV[1])
    return 0
end
redis.call('del', KEYS[1])
redis.call('publish', KEYS[2], ARGV[3])
return 1
