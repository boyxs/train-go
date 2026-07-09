-- 放弃排队：从 queue + qts 移除自己（优雅放弃，不等 deadline 逐出、不占位堵后面的人）。
-- KEYS[1]=queue(list)  KEYS[2]=qts(zset)  ARGV[1]=ownerToken
redis.call('lrem', KEYS[1], 0, ARGV[1])
redis.call('zrem', KEYS[2], ARGV[1])
return 1
