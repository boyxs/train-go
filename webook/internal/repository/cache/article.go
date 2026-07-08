package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/internal/consts"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

type ArticleCache interface {
	Get(ctx context.Context, uid int64, id int64) (domain.Article, error)
	Set(ctx context.Context, article domain.Article) error
	Del(ctx context.Context, uid int64, id int64) error
	// GetPub/SetPub/DelPub 读者端公开文章缓存（key 不含 uid）
	GetPub(ctx context.Context, id int64) (domain.Article, error)
	// MGetPub 批量取，返回 map[id]Article；缺失的 id 不在 map 里
	MGetPub(ctx context.Context, ids []int64) (map[int64]domain.Article, error)
	SetPub(ctx context.Context, article domain.Article) error
	DelPub(ctx context.Context, id int64) error
	GetFirstPage(ctx context.Context) ([]domain.Article, int64, error)
	SetFirstPage(ctx context.Context, articles []domain.Article, total int64) error
	DelFirstPage(ctx context.Context) error
}

type RedisArticleCache struct {
	cmd        redis.Cmdable
	expiration time.Duration
	l          logger.LoggerX
}

func NewRedisArticleCache(cmd redis.Cmdable, l logger.LoggerX) ArticleCache {
	return &RedisArticleCache{
		cmd:        cmd,
		expiration: consts.CacheTTL,
		l:          l,
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

// MGetPub 批量取公开文章；返回 id→Article map，缺失/反序列化失败的 id 不在结果里。
// 整体 MGET 失败才向上抛 error；个别 key miss 不算错误。
func (ac *RedisArticleCache) MGetPub(ctx context.Context, ids []int64) (map[int64]domain.Article, error) {
	if len(ids) == 0 {
		return map[int64]domain.Article{}, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = ac.getPubKey(id)
	}
	values, err := ac.cmd.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[int64]domain.Article, len(ids))
	for i, v := range values {
		if v == nil {
			continue
		}
		raw, ok := v.(string)
		if !ok {
			ac.l.Warn("MGetPub 返回值类型异常",
				logger.Int64("id", ids[i]), logger.String("type", fmt.Sprintf("%T", v)))
			continue
		}
		var article domain.Article
		if uerr := json.Unmarshal([]byte(raw), &article); uerr != nil {
			ac.l.Warn("MGetPub 反序列化失败（cache 数据损坏，下次回源 DB 修复）",
				logger.Int64("id", ids[i]), logger.Error(uerr))
			continue
		}
		result[ids[i]] = article
	}
	return result, nil
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
