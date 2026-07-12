package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// cacheKeyPattern embedding 缓存键：embedding:cache:{textHash}（sha256 前 8 字节 hex）。
// 原在 core internal/consts，随 embedding 抽到 pkg 后就近内联（本包唯一使用方）。
const cacheKeyPattern = "embedding:cache:%s"

// CachedClient 对 EmbeddingClient 加 Redis 缓存，相同 text 不重复调 API。
type CachedClient struct {
	delegate EmbeddingClient
	cmd      redis.Cmdable
	l        logger.LoggerX
	ttl      time.Duration
}

func NewCachedClient(delegate EmbeddingClient, cmd redis.Cmdable, l logger.LoggerX) EmbeddingClient {
	return &CachedClient{
		delegate: delegate,
		cmd:      cmd,
		l:        l,
		ttl:      time.Hour,
	}
}

func (c *CachedClient) Embed(ctx context.Context, text string) ([]float32, error) {
	key := c.cacheKey(text)

	// 先查缓存
	data, err := c.cmd.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		var vec []float32
		jsonErr := json.Unmarshal(data, &vec)
		if jsonErr == nil {
			return vec, nil
		}
		c.l.Warn("embedding 缓存值解析失败，回源 API", logger.String("key", key), logger.Error(jsonErr))
	case !errors.Is(err, redis.Nil):
		// 非 miss 的真实故障（连接失败等）：记日志留观测信号，仍回源 API
		c.l.Warn("读 embedding 缓存故障，回源 API", logger.String("key", key), logger.Error(err))
	}

	// miss → 调 API
	vec, err := c.delegate.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// 回填缓存（失败不影响返回）
	if buf, jsonErr := json.Marshal(vec); jsonErr == nil {
		jitter := time.Duration(rand.Int63n(int64(10 * time.Minute)))
		if setErr := c.cmd.Set(ctx, key, buf, c.ttl+jitter).Err(); setErr != nil {
			c.l.Error("回填 embedding 缓存失败",
				logger.String("key", key),
				logger.Error(setErr))
		}
	}

	return vec, nil
}

func (c *CachedClient) cacheKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf(cacheKeyPattern, hex.EncodeToString(h[:8]))
}
