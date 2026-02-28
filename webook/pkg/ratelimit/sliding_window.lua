-- 限流对象
local key = KEYS[1]
-- 窗口大小
local windowMs = tonumber(ARGV[1])
-- 阈值
local threshold = tonumber(ARGV[2])
-- 窗口起始时间
local now = tonumber(ARGV[3])
local windowStart = now - windowMs

-- 限流算法
redis.call("ZREMRANGEBYSCORE", key, "-inf", windowStart)
local cnt = redis.call("ZCOUNT", key, "-inf", "+inf")
--local cnt = redis.call("ZCOUNT", key, windowStart, "+inf")
if cnt >= threshold then
    -- 触发限流
    return "true"
else
    -- 记录请求
    redis.call("ZADD", key, now, now)
    -- 过期时间
    redis.call("PEXPIRE", key, windowMs)
    return "false"
end
