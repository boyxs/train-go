package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

type ArticleCache interface {
	Get(ctx context.Context, uid int64, id int64) (domain.Article, error)
	Set(ctx context.Context, article domain.Article) error
	Del(ctx context.Context, uid int64, id int64) error
}

type RedisArticleCache struct {
	cmd        redis.Cmdable
	expiration time.Duration
}

func NewRedisArticleCache(cmd redis.Cmdable) ArticleCache {
	return &RedisArticleCache{
		cmd:        cmd,
		expiration: consts.CacheTTL,
	}
}

func (ac *RedisArticleCache) Get(ctx context.Context, uid int64, id int64) (domain.Article, error) {
	data, err := ac.cmd.Get(ctx, ac.getKey(uid, id)).Result()
	if err != nil {
		return domain.Article{}, err
	}
	var article domain.Article
	err = json.Unmarshal([]byte(data), &article)
	return article, err
}

func (ac *RedisArticleCache) Set(ctx context.Context, article domain.Article) error {
	data, err := json.Marshal(article)
	if err != nil {
		return err
	}
	// TTL 加随机偏移（0~5min），防止缓存雪崩
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	return ac.cmd.Set(ctx, ac.getKey(article.Author.Id, article.Id), data, ac.expiration+jitter).Err()
}

func (ac *RedisArticleCache) Del(ctx context.Context, uid int64, id int64) error {
	return ac.cmd.Del(ctx, ac.getKey(uid, id)).Err()
}

func (ac *RedisArticleCache) getKey(uid int64, id int64) string {
	return fmt.Sprintf(consts.ArticlePattern, uid, id)
}
