package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/redis/go-redis/v9"
)

var ErrKeyNotExist = redis.Nil

type UserCache interface {
	Set(ctx context.Context, user domain.User) error
	Get(ctx context.Context, userid int64) (domain.User, error)
	Del(ctx context.Context, userid int64) error
}

type RedisUserCache struct {
	cmd        redis.Cmdable
	expiration time.Duration
}

func NewRedisUserCache(cmd redis.Cmdable) UserCache {
	return &RedisUserCache{
		cmd:        cmd,
		expiration: consts.CacheTTL,
	}
}

func (uc *RedisUserCache) Set(ctx context.Context, u domain.User) error {
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	jitter := time.Duration(rand.Intn(300)) * time.Second // 0~5min 随机抖动
	return uc.cmd.Set(ctx, uc.getKey(u.Id), data, uc.expiration+jitter).Err()

}

func (uc *RedisUserCache) Get(ctx context.Context, userid int64) (domain.User, error) {
	data, err := uc.cmd.Get(ctx, uc.getKey(userid)).Result()
	if err != nil {
		return domain.User{}, err
	}
	var u domain.User
	err = json.Unmarshal([]byte(data), &u)
	return u, err
}

func (uc *RedisUserCache) Del(ctx context.Context, userid int64) error {
	return uc.cmd.Del(ctx, uc.getKey(userid)).Err()
}

func (uc *RedisUserCache) getKey(userid int64) string {
	return fmt.Sprintf(consts.UserPattern, userid)
}
