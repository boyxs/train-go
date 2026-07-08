package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/service/embedding"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// CachedEmbeddingClient 对 embedding.Client 加 Redis 缓存，相同 text 不重复调 API
type CachedEmbeddingClient struct {
	delegate embedding.Client
	cmd      redis.Cmdable
	l        logger.LoggerX
	ttl      time.Duration
}

func NewCachedEmbeddingClient(delegate embedding.Client, cmd redis.Cmdable, l logger.LoggerX) embedding.Client {
	return &CachedEmbeddingClient{
		delegate: delegate,
		cmd:      cmd,
		l:        l,
		ttl:      time.Hour,
	}
}

func (c *CachedEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	key := c.cacheKey(text)

	// 先查缓存
	data, err := c.cmd.Get(ctx, key).Bytes()
	if err == nil {
		var vec []float32
		if jsonErr := json.Unmarshal(data, &vec); jsonErr == nil {
			return vec, nil
		}
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

func (c *CachedEmbeddingClient) cacheKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf(consts.EmbeddingCachePattern, hex.EncodeToString(h[:8]))
}
