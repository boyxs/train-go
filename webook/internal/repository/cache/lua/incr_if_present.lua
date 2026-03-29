local key = KEYS[1]
if redis.call('EXISTS', key) == 1 then
    redis.call('HINCRBY', key, ARGV[1], ARGV[2])
    return 1
end
return 0
