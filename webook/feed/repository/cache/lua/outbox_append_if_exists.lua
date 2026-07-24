-- outbox 条件追加：仅当发件箱已存在（已被回源填充过）时才追加。
-- 否则事件会把不存在的 outbox 创建成「只含 1 条」的假全量缓存，读时误判为完整。
-- KEYS[1]=outbox key；ARGV: [1]=score(publishedAt) [2]=member(articleId) [3]=cap [4]=ttl(ms)
local key = KEYS[1]
if redis.call('EXISTS', key) == 0 then
    return 0
end
redis.call('ZADD', key, ARGV[1], ARGV[2])
redis.call('ZREMRANGEBYRANK', key, 0, -tonumber(ARGV[3]) - 1)
redis.call('PEXPIRE', key, ARGV[4])
return 1
