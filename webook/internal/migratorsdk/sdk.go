// Package migratorsdk 是业务侧（webook-core / webook-chat 等）接入数据迁移的 SDK。
//
// 设计目标：
//
//  1. 侵入最小：业务 DAO 改造仅注入两个接口，不感知 migrator 服务
//  2. 默认零开销：NoOp 实现永远走 OLD 侧，主服务无 Redis 调用、无新路径
//  3. migrator 服务挂掉不影响主服务：SDK 只读 Redis（不调 migrator gRPC）；Redis 挂掉降级回 NoOp 兜底
//
// 启用方式：wire 注入时按 `migrator.sdk.enabled` yaml flag 切换 NoOp / Redis 实现。
package migratorsdk

import "context"

// Side 标识读写哪一侧（OLD = 源端，NEW = 目标端）。
type Side string

const (
	SideOld Side = "old"
	SideNew Side = "new"
)

// TaskName 业务调用 SDK 传给 Redis key 的迁移任务名（与 webook-migrator 控制库 task.name 一致）。
// 用 named string type 避免 wire 多个 string provider 冲突。
type TaskName string

// SwitchReader 决定一次读应该走哪一侧。
//
//	taskName  迁移任务名（与 migrator 控制库 task.name 一致）
//	hashKey   分流 hash 输入（通常用 user_id / biz_id 等保 read-your-write 不破）
type SwitchReader interface {
	ChooseSide(ctx context.Context, taskName string, hashKey int64) (Side, error)
}

// DualWriter 决定一次写应该投到哪些侧、失败如何兜底。
//
// 用法：
//
//	dualWriter.Write(ctx, "article_v1", func(side Side) error {
//	    if side == SideOld { return oldDAO.Insert(ctx, a) }
//	    return newDAO.Insert(ctx, a)
//	})
//
// fn 会被调用 1-2 次（依切流阶段决定）。
type DualWriter interface {
	Write(ctx context.Context, taskName string, fn func(side Side) error) error
}
