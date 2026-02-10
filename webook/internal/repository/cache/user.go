package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

var ErrKeyNotExist = redis.Nil

type IUserCache interface {
	Set(ctx context.Context, user domain.User) error
	Get(ctx context.Context, userid int64) (domain.User, error)
	Del(ctx context.Context, userid int64) error
}

type UserCache struct {
	cmd        redis.Cmdable
	expiration time.Duration
}

func NewUserCache(cmd redis.Cmdable) IUserCache {
	return &UserCache{
		cmd:        cmd,
		expiration: consts.CacheTTL,
	}
}

func (uc *UserCache) Set(ctx context.Context, u domain.User) error {
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	return uc.cmd.Set(ctx, getKey(u.Id), data, uc.expiration).Err()

}

func (uc *UserCache) Get(ctx context.Context, userid int64) (domain.User, error) {
	data, err := uc.cmd.Get(ctx, getKey(userid)).Result()
	if err != nil {
		return domain.User{}, err
	}
	var u domain.User
	err = json.Unmarshal([]byte(data), &u)
	return u, err
}

func (uc *UserCache) Del(ctx context.Context, userid int64) error {
	return uc.cmd.Del(ctx, getKey(userid)).Err()
}

func getKey(userid int64) string {
	return fmt.Sprintf("user:%d", userid)
}
