package redislock

// key 集中定义。{k} 花括号是 Redis Cluster hash tag：一把锁的全部 key 落同一 slot，
// 化解多 key Lua 的 CROSSSLOT；单机无副作用。{k} 为调用方原始 key。
// 示例：调用方 key "cronx:lock:ranking" → redislock:{cronx:lock:ranking}:lock。
const keyPrefix = "redislock:"

// lockKey 锁主体，hash{ownerToken: 重入计数}。
func lockKey(k string) string { return keyPrefix + "{" + k + "}:lock" }

// fenceKey 单调 fencing 计数器（string，INCR），持久 / 超长 TTL。
func fenceKey(k string) string { return keyPrefix + "{" + k + "}:fence" }

// channelKey 释放通知 channel，唤醒 pub/sub 阻塞的等待者。
func channelKey(k string) string { return keyPrefix + "{" + k + "}:ch" }

// unlockMsg 完全释放时 publish 到 channelKey 的负载，内容不重要，唤醒即可。
const unlockMsg = "released"
