package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// CachedEmbeddingClient 对 EmbeddingClient 加 Redis 缓存，相同 text 不重复调 API
type CachedEmbeddingClient struct {
	delegate EmbeddingClient
	cmd      redis.Cmdable
	ttl      time.Duration
}

func NewCachedEmbeddingClient(delegate EmbeddingClient, cmd redis.Cmdable) EmbeddingClient {
	return &CachedEmbeddingClient{
		delegate: delegate,
		cmd:      cmd,
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

	// 回填缓存（失败不影响返回，仅记日志）
	if buf, jsonErr := json.Marshal(vec); jsonErr == nil {
		jitter := time.Duration(rand.Int63n(int64(10 * time.Minute)))
		if setErr := c.cmd.Set(ctx, key, buf, c.ttl+jitter).Err(); setErr != nil {
			log.Printf("[EmbeddingCache] 回填缓存失败: key=%s, err=%v", key, setErr)
		}
	}

	return vec, nil
}

func (c *CachedEmbeddingClient) cacheKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("embed:query:%s", hex.EncodeToString(h[:8]))
}
