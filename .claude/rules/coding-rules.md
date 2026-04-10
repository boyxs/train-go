# 编码规则

## 1. 文件读取

- **同一会话内禁止重复 Read 同一文件**：首次 Read 后记住内容，后续引用之前的结果
- Read 大文件用 `offset` + `limit` 只读需要的部分，不全量读
- Edit 前必须 Read 目标文件（首次），不基于记忆或猜测修改

## 2. 文件修改

- 修改源代码（`.go` `.tsx` `.ts` `.js`）必须使用 Edit 工具，禁止 `sed -i` / `awk` / `perl -i`
- 每次 Edit 后立即 build + lint 验证，不积累多文件后验证
- 批量重命名/替换超过 3 个文件时，逐文件 Edit 并逐个验证

## 3. Wire 依赖注入

- **禁止手动编辑 `wire_gen.go`**，只修改 `wire.go`，然后运行 wire 自动生成：
  - 主入口：`cd webook && wire ./...`
  - 集成测试：`cd webook && wire ./internal/integration/setup/...`
- 修改 Provider 签名（加/删参数）、新增/删除 Provider 时，改 `wire.go` 后立即跑 wire 验证
- Provider Set 按模块组织（`searchProviderSet`、`chatProviderSet`），新模块新建 Set

## 4. Mock 生成

- **禁止手动编辑 `mocks/` 下的 mock 文件**，修改接口后运行 `make -f mk/mock.mk mockgen` 自动重新生成
- mock 文件由 mockgen 根据 source 接口自动生成，手动改了也会被覆盖

## 5. 时间存储

全链路统一 `int64`（Unix 毫秒时间戳），禁止在任何层使用 `time.Time` 存储时间字段。

**规则**：
- DAO model：`CreatedAt int64 \`gorm:"autoCreateTime:milli"\``，`UpdatedAt int64 \`gorm:"autoUpdateTime:milli"\``
- Domain model / Web VO：`int64`，直接传递，不做 `.Format()`
- 前端：`number`，用 `dayjs(timestamp).format('YYYY-MM-DD HH:mm')` 展示
- ES mapping：`"type": "date", "format": "epoch_millis"`
- MySQL 列类型：`bigint`
- DAO 内手动赋时间：`time.Now().UnixMilli()`
- 软删除：`DeletedAt soft_delete.DeletedAt \`gorm:"softDelete:milli"\``
- 新建表 `deleted_at` 列必须 `bigint NOT NULL DEFAULT 0`
- **给已有表加 `deleted_at` 列后**，必须手动执行 `UPDATE table SET deleted_at = 0 WHERE deleted_at IS NULL` + `ALTER TABLE MODIFY deleted_at bigint NOT NULL DEFAULT 0`，GORM AutoMigrate 不会自动处理已有行的 NULL 值

## 6. 缓存规则

- Cache-Aside 必须完整：读（查缓存 → miss → 查 DB → 回填）、写（写 DB → 清缓存）
- 写操作后必须清对应缓存，包括 `UpdateStatus` 等状态变更
- TTL 必须加随机 jitter（0~5min），防止缓存雪崩
- 多步 Redis 操作（如 HSet + Expire）用 Pipeline 保证原子性
- 新增 Redis 键在 `internal/consts/cache.go` 定义 Pattern

## 7. 查询性能

- 列表/分页查询用 `.Select()` 指定字段，排除大字段（Content BLOB）
- 权限校验尽量合并到 UPDATE/DELETE 的 WHERE 条件中，避免先 SELECT 再 UPDATE 的 N+1 模式
- 批量查询（如列表页查互动数据）优先走缓存，miss 再查 DB

## 8. Go 类型命名

- **所有 struct 必须导出（大写开头）**，禁止小写未导出的 struct 实现接口
- **接口用最通用的名字，实现用 `{技术}{业务}{领域}{层}` 组合前缀**：
  - 技术前缀描述实现方式：`Gorm`、`Redis`、`Cache`、`ES`
  - 业务前缀描述所属业务：`AI`、`Search` 等
  - 两者可组合，不同业务各自有独立的全链路实现
  - 例：`AIClickEventService`、`CacheAIClickEventRepository`、`GormAIClickEventDAO`、`RedisAIClickEventCache`
- 领域命名要通用化，不要绑定单一来源。例如点击追踪不叫 `AIClick`（只有 AI 场景），叫 `ClickEvent`（通用） + `Source` 字段区分来源
