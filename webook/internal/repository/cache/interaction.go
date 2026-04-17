package cache

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/redis/go-redis/v9"
)

//go:embed lua/incr_if_present.lua
var luaIncrIfPresent string

type InteractionCache interface {
	IncrReadCntIfPresent(ctx context.Context, biz string, bizId int64) error
	Get(ctx context.Context, biz string, bizId int64) (domain.Interaction, error)
	Set(ctx context.Context, intr domain.Interaction) error
	Del(ctx context.Context, biz string, bizId int64) error
	GetUserState(ctx context.Context, uid int64, biz string, bizId int64) (liked, collected bool, err error)
	SetUserState(ctx context.Context, uid int64, biz string, bizId int64, liked, collected bool) error
	DelUserState(ctx context.Context, uid int64, biz string, bizId int64) error
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
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	pipe := c.cmd.Pipeline()
	pipe.HSet(ctx, key,
		"read_cnt", intr.ReadCount,
		"like_cnt", intr.LikeCount,
		"collect_cnt", intr.CollectCount,
	)
	pipe.Expire(ctx, key, consts.InteractionTTL+jitter)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisInteractionCache) Del(ctx context.Context, biz string, bizId int64) error {
	return c.cmd.Del(ctx, c.key(biz, bizId)).Err()
}

func (c *RedisInteractionCache) key(biz string, bizId int64) string {
	return fmt.Sprintf(consts.InteractionPattern, biz, bizId)
}

func (c *RedisInteractionCache) stateKey(uid int64, biz string, bizId int64) string {
	return fmt.Sprintf(consts.InteractionStatePattern, biz, bizId, uid)
}

// GetUserState 查用户对指定业务的 liked/collected 状态
// 未命中返回 redis.Nil
func (c *RedisInteractionCache) GetUserState(ctx context.Context, uid int64, biz string, bizId int64) (bool, bool, error) {
	data, err := c.cmd.HGetAll(ctx, c.stateKey(uid, biz, bizId)).Result()
	if err != nil {
		return false, false, err
	}
	if len(data) == 0 {
		return false, false, redis.Nil
	}
	liked := data["liked"] == "1"
	collected := data["collected"] == "1"
	return liked, collected, nil
}

func (c *RedisInteractionCache) SetUserState(ctx context.Context, uid int64, biz string, bizId int64, liked, collected bool) error {
	key := c.stateKey(uid, biz, bizId)
	jitter := time.Duration(rand.Int63n(int64(5 * time.Minute)))
	pipe := c.cmd.Pipeline()
	pipe.HSet(ctx, key,
		"liked", boolStr(liked),
		"collected", boolStr(collected),
	)
	pipe.Expire(ctx, key, consts.InteractionTTL+jitter)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisInteractionCache) DelUserState(ctx context.Context, uid int64, biz string, bizId int64) error {
	return c.cmd.Del(ctx, c.stateKey(uid, biz, bizId)).Err()
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
