// Package dsn 抽象 task → MySQL 连接的解析逻辑。
//
// 设计动机：架构上 Source/Sink 应按 task.SourceDsnRef / SinkDsnRef 拿到独立的连接，
// 而不是所有 task 共用 ioc 注入的控制库 *gorm.DB。真生产实现需要：
//   - Vault / K8s Secret 解析 *DsnRef → 拿到明文 DSN
//   - 连接池缓存（按 dsn 字符串 LRU），避免每次 BuildXxx 都新建 *gorm.DB
//   - 健康检查 + 失败回退
//
// 当前包提供：
//   - Resolver 接口（src / dst 双侧解析）
//   - StaticResolver 占位实现：src/dst 都返回构造时传入的固定 db（本地演示 / 测试用）
//
// 真生产实现作为 TODO 留口：实现 Resolver 接口 + ioc 装配替换 StaticResolver 即可，
// 上层（SourceFactory / SinkFactory）无感知。
package dsn

import (
	"context"

	"gorm.io/gorm"

	"github.com/webook/migrator/domain"
)

// Resolver 把 task 解析为 src / dst 端的 *gorm.DB 连接。
//
// SourceFactory / SinkFactory 持有 Resolver；BuildXxx 调对应 Resolve 方法拿连接，
// 不再绑死 ioc 注入的单一 db。
type Resolver interface {
	ResolveSrc(ctx context.Context, task domain.Task) (*gorm.DB, error)
	ResolveDst(ctx context.Context, task domain.Task) (*gorm.DB, error)
}

// StaticResolver 占位实现：src/dst 都返回构造时传入的固定 db。
//
// 用法：本地演示模式 src/dst 在同一 MySQL 实例的不同 schema 或同 schema 不同表。
// 真生产替换为按 task.SourceDsnRef / SinkDsnRef 解析 + Vault / K8s Secret 拿密码的实现。
//
// TODO(prod): 实现 PerTaskResolver — 维护 dsn → *gorm.DB 的 LRU 缓存（容量 ~ task 总数），
// 按 task.SourceDsnRef / SinkDsnRef 查 Vault 拿明文 DSN，gorm.Open 建连后缓存复用。
type StaticResolver struct {
	db *gorm.DB
}

// NewStaticResolver 构造一个 src/dst 共用 db 的 Resolver（占位 / 本地用）。
func NewStaticResolver(db *gorm.DB) Resolver {
	return &StaticResolver{db: db}
}

func (r *StaticResolver) ResolveSrc(_ context.Context, _ domain.Task) (*gorm.DB, error) {
	return r.db, nil
}

func (r *StaticResolver) ResolveDst(_ context.Context, _ domain.Task) (*gorm.DB, error) {
	return r.db, nil
}
