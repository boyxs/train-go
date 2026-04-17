package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
)

type ArticleCache interface {
	Get(ctx context.Context, uid int64, id int64) (domain.Article, error)
	Set(ctx context.Context, article domain.Article) error
	Del(ctx context.Context, uid int64, id int64) error
	// GetPub/SetPub/DelPub 读者端公开文章缓存（key 不含 uid）
	GetPub(ctx context.Context, id int64) (domain.Article, error)
	SetPub(ctx context.Context, article domain.Article) error
	DelPub(ctx context.Context, id int64) error
	GetFirstPage(ctx context.Context) ([]domain.Article, int64, error)
	SetFirstPage(ctx context.Context, articles []domain.Article, total int64) error
	DelFirstPage(ctx context.Context) error
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

// firstPageData 首页缓存结构，包含列表和总数
type firstPageData struct {
	Articles []domain.Article `json:"articles"`
	Total    int64            `json:"total"`
}

func (ac *RedisArticleCache) GetFirstPage(ctx context.Context) ([]domain.Article, int64, error) {
	data, err := ac.cmd.Get(ctx, consts.ReaderFirstPageKey).Result()
	if err != nil {
		return nil, 0, err
	}
	var page firstPageData
	err = json.Unmarshal([]byte(data), &page)
	return page.Articles, page.Total, err
}

func (ac *RedisArticleCache) SetFirstPage(ctx context.Context, articles []domain.Article, total int64) error {
	// 拷贝一份，清空 Content 减小体积，不污染调用方数据
	cp := make([]domain.Article, len(articles))
	copy(cp, articles)
	for i := range cp {
		cp[i].Content = ""
	}
	data, err := json.Marshal(firstPageData{Articles: cp, Total: total})
	if err != nil {
		return err
	}
	jitter := time.Duration(rand.Int63n(int64(time.Minute)))
	return ac.cmd.Set(ctx, consts.ReaderFirstPageKey, data, consts.FirstPageTTL+jitter).Err()
}

func (ac *RedisArticleCache) DelFirstPage(ctx context.Context) error {
	return ac.cmd.Del(ctx, consts.ReaderFirstPageKey).Err()
}

func (ac *RedisArticleCache) GetPub(ctx context.Context, id int64) (domain.Article, error) {
	data, err := ac.cmd.Get(ctx, ac.getPubKey(id)).Result()
	if err != nil {
		return domain.Article{}, err
	}
	var article domain.Article
	err = json.Unmarshal([]byte(data), &article)
	return article, err
}

func (ac *RedisArticleCache) SetPub(ctx context.Context, article domain.Article) error {
	data, err := json.Marshal(article)
	if err != nil {
		return err
	}
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	return ac.cmd.Set(ctx, ac.getPubKey(article.Id), data, ac.expiration+jitter).Err()
}

func (ac *RedisArticleCache) DelPub(ctx context.Context, id int64) error {
	return ac.cmd.Del(ctx, ac.getPubKey(id)).Err()
}

func (ac *RedisArticleCache) getKey(uid int64, id int64) string {
	return fmt.Sprintf(consts.ArticlePattern, uid, id)
}

func (ac *RedisArticleCache) getPubKey(id int64) string {
	return fmt.Sprintf(consts.ArticlePubPattern, id)
}
