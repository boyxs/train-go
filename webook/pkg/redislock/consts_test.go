package redislock

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hashTag 提取 Redis Cluster hash tag（首个 {...} 之间内容）；空 tag `{}` 或无闭合 → ""（整键哈希）。
func hashTag(key string) string {
	i := strings.IndexByte(key, '{')
	if i < 0 {
		return ""
	}
	j := strings.IndexByte(key[i+1:], '}')
	if j <= 0 { // 空 tag {} 或无闭合 } → 无有效 tag
		return ""
	}
	return key[i+1 : i+1+j]
}

// 同一把锁的全部 key（lock/fence/ch/queue/qts）必须落同一非空 hash tag → 集群同 slot、多 key
// Lua（release/fencing/fair）不 CROSSSLOT。守卫回归：将来新增第 6 种 key 忘了 {k} 包裹即挂。
func TestKeys_SameSlotHashTag(t *testing.T) {
	for _, k := range []string{"cronx:lock:ranking", "a", "user:42:balance", "含中文的 key", "a}b{c"} {
		builders := map[string]string{
			"lock":    lockKey(k),
			"fence":   fenceKey(k),
			"channel": channelKey(k),
			"queue":   queueKey(k),
			"qts":     qtsKey(k),
		}
		want := hashTag(lockKey(k))
		require.NotEmptyf(t, want, "key %q 的 hash tag 不应为空", k)
		for name, key := range builders {
			assert.Equalf(t, want, hashTag(key), "key=%q 的 %s builder hash tag 应与 lock 一致（同 slot）", k, name)
		}
	}
}
