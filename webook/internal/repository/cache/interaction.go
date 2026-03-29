package cache

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

//go:embed lua/incr_if_present.lua
var luaIncrIfPresent string

type InteractionCache interface {
	IncrReadCntIfPresent(ctx context.Context, biz string, bizId int64) error
	Get(ctx context.Context, biz string, bizId int64) (domain.Interaction, error)
	Set(ctx context.Context, intr domain.Interaction) error
	Del(ctx context.Context, biz string, bizId int64) error
}

type RedisInteractionCache struct {
	cmd redis.Cmdable
}

func NewRedisInteractionCache(cmd redis.Cmdable) InteractionCache {
	return &RedisInteractionCache{cmd: cmd}
}

func (c *RedisInteractionCache) IncrReadCntIfPresent(ctx context.Context, biz string, bizId int64) error {
	return c.cmd.Eval(ctx, luaIncrIfPresent,
		[]string{c.key(biz, bizId)},
		"read_cnt", 1,
	).Err()
}

func (c *RedisInteractionCache) Get(ctx context.Context, biz string, bizId int64) (domain.Interaction, error) {
	data, err := c.cmd.HGetAll(ctx, c.key(biz, bizId)).Result()
	if err != nil {
		return domain.Interaction{}, err
	}
	if len(data) == 0 {
		return domain.Interaction{}, redis.Nil
	}
	readCnt, _ := strconv.ParseInt(data["read_cnt"], 10, 64)
	likeCnt, _ := strconv.ParseInt(data["like_cnt"], 10, 64)
	collectCnt, _ := strconv.ParseInt(data["collect_cnt"], 10, 64)
	return domain.Interaction{
		BizId:        bizId,
		Biz:          biz,
		ReadCount:    readCnt,
		LikeCount:    likeCnt,
		CollectCount: collectCnt,
	}, nil
}

func (c *RedisInteractionCache) Set(ctx context.Context, intr domain.Interaction) error {
	key := c.key(intr.Biz, intr.BizId)
	err := c.cmd.HSet(ctx, key,
		"read_cnt", intr.ReadCount,
		"like_cnt", intr.LikeCount,
		"collect_cnt", intr.CollectCount,
	).Err()
	if err != nil {
		return err
	}
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	return c.cmd.Expire(ctx, key, consts.InteractionTTL+jitter).Err()
}

func (c *RedisInteractionCache) Del(ctx context.Context, biz string, bizId int64) error {
	return c.cmd.Del(ctx, c.key(biz, bizId)).Err()
}

func (c *RedisInteractionCache) key(biz string, bizId int64) string {
	return fmt.Sprintf(consts.InteractionPattern, biz, bizId)
}
