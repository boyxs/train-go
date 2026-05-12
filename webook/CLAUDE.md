# Webook 后端

Go + Gin + GORM + Redis + Wire（DI）

模块路径: `github.com/webook`

## 常用命令

```bash
go build ./...                        # 编译
go vet ./...                          # 静态分析
go test ./...                         # 全量测试
go test ./internal/integration/...    # 集成测试（需 MySQL + Redis）
wire ./...                            # 重新生成主 wire_gen.go
wire ./internal/integration/setup/... # 重新生成集成测试 wire_gen.go
make -f mk/mock.mk mockgen           # 重新生成 Mock
make -f mk/es.mk help                # ES 管理命令
make -f mk/infra.mk help             # 基础设施管理

# 启动示例（默认用 config/local.yaml）
APP_ENV=config/local.yaml go run main.go
```

## 环境说明

应用配置命名标准（**config 层**，不同于部署层 `.env.<env>`）：

| 文件 | 角色 | 谁用 | 特征 |
|------|------|------|------|
| `config/local.yaml` | 本地开发 | 你 Windows `go run`、IDE 调试 | 明文密码连本机 docker compose |
| `config/dev.yaml` | 团队共享 dev / CI 集成测试 | 部署 `webook-dev` project | `otel.env=dev`, `sampleRatio=1.0`, logger debug |
| `config/staging.yaml` | 预发布 | 部署 `webook-staging` project | `otel.env=staging`, `sampleRatio=0.5` |
| `config/prod.yaml` | 生产 | 部署 `webook-prod` project | `otel.env=prod`, `sampleRatio=0.1`, logger info |

**配置方案**（L1 学习项目）：
- 四份 yaml 同构但按环境差异化（otel.env / sampleRatio / logger / 密码等）
- 密码/API key 直接写在 yaml 里（`mysql.dsn` / `redis.password` / `llm.providers[].apiKey`）
- 应用进 docker 部署时，`.env.<env>` 的 `APP_ENV` 决定加载哪份 yaml
- `.env.<env>` 的 `MYSQL_PASS` / `REDIS_PASS` 必须和 yaml 里的密码一致（中间件容器起动用，不同步会连不上）

**docker-compose 动态选 yaml**：`APP_ENV: "${APP_ENV}"`，`.env.<env>` 里填 `config/<env>.yaml`。

**L2 K8s 演进**：yaml 进 ConfigMap，密码剥到 K8s Secret，`envFrom.secretRef` 注入容器环境变量，应用侧加 `viper.BindEnv("mysql.dsn", "MYSQL_DSN")` 显式绑定 env key → yaml 字段（纯 AutomaticEnv 对嵌套 key 不生效已实测验证）。

**L1 部署层**（`deploy/` 目录，项目部署唯一真相源）：
- `docker-compose.yaml` + 四份 `.env.<env>`（local/dev/staging/prod）
- `./deploy.sh local`：本地开发（build 代码 + 暴露宿主端口给 go run / DBeaver）
- `./deploy.sh <dev|staging|prod>`：服务器部署（pull ghcr 镜像）
- 同时只跑一套（container_name 唯一），volume 按 project 隔离（`webook-<env>_*`）不串
- K8s 走仓库根 `kubernetes/` 目录（已建），后续如引入 helm 再新开 `helm/`，均与 `deploy/` 并列，不挤压这里

完整 CI/CD 演进路线见 `C:\Go\notes\cicd-webook-roadmap.md`。

## 导航

```
Handler (web/) → Service (service/) → Repository (repository/) → DAO (dao/) / Cache (cache/)
```

| 目录 | 职责 | 找什么来这里 |
|------|------|------------|
| `internal/web/` | 路由、参数绑定、响应 | API 入口、请求/响应结构 |
| `internal/service/` | 业务逻辑 | 业务规则、校验 |
| `internal/repository/` | 协调 DAO + Cache | 数据访问逻辑 |
| `internal/repository/dao/` | GORM 模型和查询 | 表结构、SQL |
| `internal/repository/cache/` | Redis 缓存 | 缓存键、TTL |
| `internal/domain/` | 领域模型 | 业务实体定义 |
| `internal/consts/` | 共享常量 | Token Key、TTL、Redis 键模式 |
| `internal/service/ai/` | LLM 聊天（流式） | LLMClient 接口、Failover、TimeoutFailover |
| `internal/service/ai/embedding/` | 文本向量化 | EmbeddingClient 接口、OpenAI/Ollama/Failover |
| `pkg/` | 跨项目工具 | 限流器、日志接口、Gin 中间件 |
| `ioc/` | Wire Provider | 基础设施初始化 |
| `config/` | 环境配置 | YAML 配置项 |
| `wire.go` | 主入口 DI | 依赖注入全景 |

## 分层规则

- 依赖方向严格单向：Handler → Service → Repository → DAO/Cache，**禁止跨层调用**
- `domain` 是最内层，所有层可依赖它，但 **DAO 不可依赖 domain**
- 每层通过接口解耦，Wire 负责注入实现
- Repository 层实现 Cache-Aside：查询先缓存 → miss 回源 DAO → 回填；写入后清缓存
- 事务在 DAO 层（`db.Transaction()`），Service 层不感知数据库细节
- **缓存只能在 `cache/` 层**：Service 层禁止直接操作 Redis（包括装饰器缓存）
- **子包按能力拆分**：同一目录下两种独立能力（如 LLM 聊天 vs 文本向量化）必须拆子包，不平铺
- **接口命名避免歧义**：子包内接口用完整名（`EmbeddingClient` 而非 `Client`），调用方读 `embedding.EmbeddingClient` 一眼能分清
- **Handler / Service / Repository / DAO 构造函数必须返回接口**，不返回具体指针；实现用 `Internal` 或技术前缀（`InternalChatHandler` / `AIClickEventHandler` / `GormArticleAuthorDAO`）。入参也依赖接口，方便 Wire 注入和单测 mock。

## 命名规范

| 维度 | 规则 | 示例 |
|------|------|------|
| 接口 | `[实体][业务角色][层]` | `ArticleAuthorDAO` |
| 实现 | `[技术限定][实体][业务角色][层]` | `GormArticleAuthorDAO` |
| 表名 | **单数**，下划线分词 | `article` / `published_article` / `user_interaction`（❌ `articles`） |
| 文件 | 按业务角色，不按技术 | `article_author.go`（非 `article_dao.go`） |
| receiver | 类型首字母小写 | `func (s *chatService) Send()` |
| 查询方法 | `Find`(单条) `Page`(分页) `List`(列表) | `FindByXx` / `PageXxs` / `ListXxs` |
| 写入方法 | 按业务动作 | `Publish` / `Withdraw`（非 `Process`） |
| DB 查询结果 | `xxxList` | `articleList, err := dao.ListByAuthor()` |
| 自定义切片 | `xxxs` | `ids := make([]int64, 0, len(articleList))` |
| 索引/映射 | `xxxMap` | `authorMap := make(map[int64]Author)` |
| 单条查询 | 原名或无后缀 | `article, err := dao.FindById()` |

## 数据表规范

新建表或改表结构时，**CREATE TABLE 必须满足以下全部项**，否则 review 直接打回：

1. **表名**：单数 + 下划线（`article` / `user_interaction`，见命名规范表）
2. **表级注释**：`ENGINE=... ROW_FORMAT=Dynamic COMMENT='业务说明'`，一句话讲清这张表存什么
3. **每列必须有 COMMENT**：短句即可，重点讲枚举值含义 / 单位 / 业务约束
   - 例：`status tinyint COMMENT '状态：1=未发表 2=已发表 3=仅自己可见'`
   - 例：`created_at bigint COMMENT '创建时间（Unix 毫秒）'`
4. **时间字段**：统一 `bigint`（Unix 毫秒戳），不用 `datetime` / `timestamp`。对应 GORM `autoCreateTime:milli` / `autoUpdateTime:milli`
5. **软删除**：`deleted_at bigint NOT NULL DEFAULT 0`，0=未删；对应 GORM `softDelete:milli`
6. **字符集**：默认 `utf8mb4` + `utf8mb4_0900_ai_ci`（MySQL 8），字段需 CJK 的显式指定
7. **索引命名**（**必须带 table 前缀**，避免跨表索引名撞车）：
   - 唯一索引：`uni_{table}_{field}`（单字段）/ `uk_{table}_{业务语义}`（复合，如 `uk_ai_click_events_dedup`）
   - 普通索引：`idx_{table}_{field1}[_{field2}...]`
   - **豁免**：规则添加前已建好的索引可保持现状，不强制迁移；新建表 / 新加索引严格遵守
8. **主键**：`id bigint NOT NULL AUTO_INCREMENT COMMENT '主键'`
9. **Go 对应**：新表 struct 要加 `TableName()` 方法写明表名；字段 tag `gorm:"type:xxx;..."` 与 DDL 对齐
10. **脚本同步**：所有表结构改动必须同步到 `webook/scripts/webook.sql`（含列注释），不止改 Go struct 让 AutoMigrate 发挥；视图 `v_xxx` 也同步更新

**DDL 模板**（最小可复制）：
```sql
DROP TABLE IF EXISTS `xxx`;
CREATE TABLE `xxx` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz_field` varchar(32) NOT NULL DEFAULT '' COMMENT '字段说明',
  `created_at` bigint NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_xxx_biz_field` (`biz_field` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '一句话讲这张表';
```

## 复用已有工具包（禁止重复造轮子）

新模块开发前必须检查以下已有封装，**直接复用，不要手写替代实现**：

| 能力 | 位置 | 用法 |
|------|------|------|
| **限流** | `pkg/ratelimit/` | 注入 `Limiter` 接口，ioc 层配置窗口和阈值 |
| **Gin 限流中间件** | `pkg/ginx/middleware/ratelimit/` | IP 级别限流 |
| **日志** | `pkg/logger/` | 注入 `LoggerX` 接口，结构化字段 |
| **JWT 认证** | `internal/web/jwt/` | 双 Token，Handler 内嵌 `JwtHandler` |
| **统一响应** | `internal/web/result.go` | `Result{Code, Msg, Data}` |
| **共享常量** | `internal/consts/` | Token Key、TTL、Redis 键模式、时间格式 |

**规则：**
1. Handler 层不直接操作 Redis — 限流、缓存等通过注入的接口完成
2. Repository 层有 Cache 接口就必须用 — 不能绕过缓存直连 DAO
3. 新增 Redis 键在 `internal/consts/` 定义 Pattern，不散写 `fmt.Sprintf`
4. 通用工具函数提取到 `pkg/` 或 `internal/domain/`

## 编码约束

- 日志用注入的 `LoggerX`，禁止 `fmt.Println`
- 错误必须处理，禁止 `_ = err`
- 测试假数据用 `// ===== TODO: 测试假数据 START/END =====` 包裹
- 大数据量查询优先在数据库内完成过滤和聚合，避免全量加载到内存

## 提交前必跑 goimports

CI 用 `goimports -local github.com/webook -l .` 检查 import 顺序，乱了直接 fail。**任何 commit 前必须在 `webook/` 跑一次原生命令**：

```bash
cd webook && goimports -local github.com/webook -w .
```

把本地 import 自动分组到第三组，避免 CI 红 + 二次 commit。`workflow:done` 流水 build/lint 之前必须先跑这条命令再继续验证。

**禁止套 `make fmt`**——直接调底层二进制，规则透明可审计，不依赖 Makefile 黑盒。
