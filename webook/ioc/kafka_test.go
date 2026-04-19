package ioc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRetryConnect_SucceedsOnEventualSuccess 重试若干次后成功时：返回该值，attempts 等于实际调用次数
func TestRetryConnect_SucceedsOnEventualSuccess(t *testing.T) {
	attempts := 0
	v, ok := retryConnect("test", 5, time.Millisecond, func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, errors.New("fail")
		}
		return 42, nil
	})
	assert.True(t, ok)
	assert.Equal(t, 42, v)
	assert.Equal(t, 3, attempts)
}

// TestRetryConnect_ExhaustsAndReturnsFalse 所有尝试都失败：返回零值 + false，attempts 等于 max
func TestRetryConnect_ExhaustsAndReturnsFalse(t *testing.T) {
	attempts := 0
	v, ok := retryConnect("test", 4, time.Millisecond, func() (int, error) {
		attempts++
		return 0, errors.New("always fail")
	})
	assert.False(t, ok)
	assert.Equal(t, 0, v)
	assert.Equal(t, 4, attempts)
}

// TestRetryConnect_ExponentialBackoff 验证 backoff 翻倍（总耗时至少 1+2+4 = 7 单位）
func TestRetryConnect_ExponentialBackoff(t *testing.T) {
	start := time.Now()
	_, ok := retryConnect("test", 4, 20*time.Millisecond, func() (int, error) {
		return 0, errors.New("fail")
	})
	elapsed := time.Since(start)
	assert.False(t, ok)
	// 4 次调用之间有 3 次 sleep：20ms + 40ms + 80ms = 140ms（不含第 4 次之后，因为放弃）
	assert.GreaterOrEqual(t, elapsed, 140*time.Millisecond, "elapsed=%v 应 >= 140ms", elapsed)
}
