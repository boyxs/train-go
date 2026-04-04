# 编码规则

## 1. 文件修改

- 修改源代码（`.go` `.tsx` `.ts` `.js`）必须使用 Edit 工具，禁止 `sed -i` / `awk` / `perl -i` 等文本替换
- Edit 前必须 Read 目标文件，不基于记忆或猜测修改
- 每次 Edit 后立即 build + lint 验证，不积累多文件后验证
- 批量重命名/替换超过 3 个文件时，逐文件 Edit 并逐个验证

## 2. Wire 依赖注入

- **禁止手动编辑 `wire_gen.go`**，只修改 `wire.go`，然后运行 wire 自动生成：
  - 主入口：`cd webook && wire ./...`
  - 集成测试：`cd webook && wire ./internal/integration/setup/...`
- 修改 Provider 签名（加/删参数）、新增/删除 Provider 时，改 `wire.go` 后立即跑 wire 验证
- Provider Set 按模块组织（`searchProviderSet`、`chatProviderSet`），新模块新建 Set

## 3. Mock 生成

- **禁止手动编辑 `mocks/` 下的 mock 文件**，修改接口后运行 `make -f mk/mock.mk mockgen` 自动重新生成
- mock 文件由 mockgen 根据 source 接口自动生成，手动改了也会被覆盖

## 4. 时间存储

全链路统一 `int64`（Unix 毫秒时间戳），禁止在任何层使用 `time.Time` 存储时间字段。

**原因**：`time.Time` + MySQL `datetime` + `loc=UTC` 组合导致展示时差 8 小时，且 `.Format()` 输出裸字符串无时区标识，前端无法区分 UTC/CST。int64 时间戳无时区歧义，全链路零转换。

**规则**：
- DAO model：`CreatedAt int64 \`gorm:"autoCreateTime:milli"\``，`UpdatedAt int64 \`gorm:"autoUpdateTime:milli"\``
- Domain model：`CreatedAt int64`、`UpdatedAt int64`
- Web VO：`int64`，直接传递，不做 `.Format()`
- 前端 types：`number`，用 `dayjs(timestamp).format('YYYY-MM-DD HH:mm')` 展示
- ES mapping：`"type": "date", "format": "epoch_millis"`
- MySQL 列类型：`bigint`
- DAO 内手动赋时间：`time.Now().UnixMilli()`，不用 `time.Now()`
- 软删除：`DeletedAt soft_delete.DeletedAt \`gorm:"softDelete:milli"\``（`gorm.io/plugin/soft_delete`）
- 新建表 `deleted_at` 列必须 `bigint NOT NULL DEFAULT 0`（GORM 软删除查 `WHERE deleted_at = 0`，NULL 会导致查不到）

## 4. antd 组件使用

- `message` / `notification` / `modal` 必须通过 `App.useApp()` 获取实例，禁止静态导入
- 布局层（layout.tsx）必须包裹 `<ConfigProvider><App>...</App></ConfigProvider>`
- 主题色通过 ConfigProvider `token.colorPrimary` 统一控制，组件内不硬编码色值
- 手动用色时通过 ConfigProvider 的 `theme.token` 获取，保持全局一致

## 5. 破坏性操作

- 删除、撤回等不可逆操作统一 `Modal.confirm` 弹窗确认
- 删除最后一页最后一条数据时自动回退上一页
- 弹窗 `okButtonProps: { danger: true }`，明确标识风险

## 6. 响应式

- 移动端优先：默认样式 = 手机，通过 `md:` 断点适配桌面
- Table 在移动端替换为卡片列表（`hidden md:block` / `block md:hidden`）
- Header 移动端用 Drawer 抽屉替代水平 Menu

## 7. API 调用

- 禁止组件内直接调 `axios`，必须通过 `api/` 层
- 返回类型必须标注：`axios.post<Result<T>>()`
- 命名规范：业务函数带实体名（`findArticle`），认证函数不带（`login`）

## 8. 缓存规则

- Cache-Aside 必须完整：读（查缓存 → miss → 查 DB → 回填）、写（写 DB → 清缓存）
- 写操作后必须清对应缓存，包括 `UpdateStatus` 等状态变更
- TTL 必须加随机 jitter（0~5min），防止缓存雪崩
- 多步 Redis 操作（如 HSet + Expire）用 Pipeline 保证原子性
- 新增 Redis 键在 `internal/consts/cache.go` 定义 Pattern

## 9. 查询性能

- 列表/分页查询用 `.Select()` 指定字段，排除大字段（Content BLOB）
- 权限校验尽量合并到 UPDATE/DELETE 的 WHERE 条件中，避免先 SELECT 再 UPDATE 的 N+1 模式
- 批量查询（如列表页查互动数据）优先走缓存，miss 再查 DB
