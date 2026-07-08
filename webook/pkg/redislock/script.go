package redislock

import (
	_ "embed"

	"github.com/redis/go-redis/v9"
)

// Lua 集中 embed。全部脚本原子执行，hash 存储模型 + 可重入计数（§3.1）。

//go:embed scripts/acquire.lua
var acquireLua string

//go:embed scripts/release.lua
var releaseLua string

//go:embed scripts/refresh.lua
var refreshLua string

//go:embed scripts/fence.lua
var fenceLua string

var (
	// acquireScript 获取 / 重入：-1=成功；>=0=被占（剩余 pttl 作等待提示）。
	acquireScript = redis.NewScript(acquireLua)
	// releaseScript 释放：-1=不在我手里；0=重入未归零仍持有；1=完全释放。
	releaseScript = redis.NewScript(releaseLua)
	// refreshScript 续约：1=成功；0=不在我手里。
	refreshScript = redis.NewScript(refreshLua)
	// fenceScript 带 fencing 的获取，返回 {status, fence}（§3.3）。
	fenceScript = redis.NewScript(fenceLua)
)
