-- 续约：1=成功；0=不在我手里（token 不匹配 / 已过期）。
-- KEYS[1]=redislock:{k}:lock  ARGV[1]=leaseMs  ARGV[2]=ownerToken
if redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
    redis.call('pexpire', KEYS[1], ARGV[1])
    return 1
end
return 0
