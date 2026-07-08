package migratorsdk

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/redis/go-redis/v9"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// Redis key 前缀（与 webook-migrator service/switching 同源）。
const (
	keyStage = "migrator:stage:"
	keyGray  = "migrator:gray:"
)

// stage 枚举（与 migrator domain.Stage 同源 string 值）。
const (
	stageSrcOnly  = "SRC_ONLY"
	stageSrcFirst = "SRC_FIRST"
	stageDstFirst = "DST_FIRST"
	stageDstOnly  = "DST_ONLY"
)

// RedisSwitchReader 按 Redis 中的 stage + gray 决策读路由。
//
// 行为：
//
//	stage = SRC_ONLY (默认 / 未配置)     → SideOld
//	stage = SRC_FIRST + gray%            → hash(hashKey) % 100 < gray% 时 SideNew，否则 SideOld
//	stage = DST_FIRST / DST_ONLY         → SideNew
//
// Redis 故障 → 降级 SideOld（保业务可用，不抛错）。
type RedisSwitchReader struct {
	cmd redis.Cmdable
	l   logger.LoggerX
}

func NewRedisSwitchReader(cmd redis.Cmdable, l logger.LoggerX) SwitchReader {
	return &RedisSwitchReader{cmd: cmd, l: l}
}

func (r *RedisSwitchReader) ChooseSide(ctx context.Context, taskName string, hashKey int64) (Side, error) {
	stage, err := r.cmd.Get(ctx, keyStage+taskName).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		r.l.Warn("migratorsdk read stage failed, fallback to old",
			logger.String("task", taskName), logger.Error(err))
		return SideOld, nil
	}
	switch stage {
	case stageDstFirst, stageDstOnly:
		return SideNew, nil
	case stageSrcFirst:
		gray, gerr := r.cmd.Get(ctx, keyGray+taskName).Int()
		if gerr != nil {
			return SideOld, nil
		}
		if gray <= 0 {
			return SideOld, nil
		}
		if gray >= 100 {
			return SideNew, nil
		}
		if hashMod100(hashKey) < uint32(gray) {
			return SideNew, nil
		}
		return SideOld, nil
	default:
		return SideOld, nil
	}
}

// FailureRecorder 双写 NEW 侧失败时的兜底回调（用于落 dead_letter 表 / 发告警 / 等）。
//
// SDK 不强制依赖具体表实现 — 业务方可选注入；不注入则失败只 log warn。
// 这避免 SDK import migrator/repository/dao 造成跨服务耦合。
type FailureRecorder interface {
	Record(ctx context.Context, taskName string, cause error)
}

// NoOpFailureRecorder 仅 log warn 不落表；适合 demo / 单测。
type NoOpFailureRecorder struct{ L logger.LoggerX }

func (n NoOpFailureRecorder) Record(_ context.Context, taskName string, cause error) {
	if n.L != nil {
		n.L.Warn("migratorsdk dual write NEW failed",
			logger.String("task", taskName), logger.Error(cause))
	}
}

// RedisDualWriter 按 stage 决定写策略；NEW 侧失败调 FailureRecorder 兜底。
//
// 阶段行为：
//
//	SRC_ONLY  / 空      只写 OLD（fn 调一次）
//	SRC_FIRST           OLD 必成 → NEW 尽力（失败调 Recorder，业务不报错）
//	DST_FIRST           OLD + NEW 都必成（任一失败业务报错）
//	DST_ONLY            只写 NEW（fn 调一次）
//
// stage 读 Redis 失败降级 SRC_ONLY（最保守 = 业务行为不变）。
type RedisDualWriter struct {
	cmd      redis.Cmdable
	recorder FailureRecorder
	l        logger.LoggerX
}

func NewRedisDualWriter(cmd redis.Cmdable, recorder FailureRecorder, l logger.LoggerX) DualWriter {
	if recorder == nil {
		recorder = NoOpFailureRecorder{L: l}
	}
	return &RedisDualWriter{cmd: cmd, recorder: recorder, l: l}
}

func (w *RedisDualWriter) Write(ctx context.Context, taskName string, fn func(side Side) error) error {
	stage, err := w.cmd.Get(ctx, keyStage+taskName).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		w.l.Warn("migratorsdk read stage failed, fallback to SRC_ONLY",
			logger.String("task", taskName), logger.Error(err))
		stage = stageSrcOnly
	}
	switch stage {
	case stageDstOnly:
		return fn(SideNew)
	case stageDstFirst:
		if oerr := fn(SideOld); oerr != nil {
			return fmt.Errorf("dual write OLD: %w", oerr)
		}
		if nerr := fn(SideNew); nerr != nil {
			return fmt.Errorf("dual write NEW: %w", nerr)
		}
		return nil
	case stageSrcFirst:
		if oerr := fn(SideOld); oerr != nil {
			return oerr
		}
		if nerr := fn(SideNew); nerr != nil {
			w.recorder.Record(ctx, taskName, nerr)
		}
		return nil
	default:
		return fn(SideOld)
	}
}

// hashMod100 用 FNV-1a hash → uint32 → % 100；保 hashKey 同值始终落同一灰度桶。
// 用 hash 而非裸 hashKey%100 因为 hashKey 经常是连续 ID（如 user_id 顺序递增），直接取模分布不均。
func hashMod100(hashKey int64) uint32 {
	h := fnv.New32a()
	var buf [8]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(hashKey >> (i * 8))
	}
	_, _ = h.Write(buf[:])
	return h.Sum32() % 100
}
