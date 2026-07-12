# Webook 后端

Go + Gin + GORM + Redis + Wire（DI）

模块路径: `github.com/boyxs/train-go/webook`（多模块 go.work：各服务 + pkg/api/shared 各自独立 module，无 root go.mod）

> **模块消费**：仓内走 `replace ../x` 本地解析、不经 go get；对外/跨仓 `go get` 本仓（私有）模块需 `GOPRIVATE=github.com/boyxs/*` + git 认证。模块版本 tag 用目录前缀式 `webook/<mod>/vX.Y.Z`（如 `webook/pkg/v1.0.0`），别和服务部署 tag `webook-<svc>-v*` 混。详见 `prd/go-workspace/ARCHITECTURE.md` §10。

## 常用命令

```bash
# 多模块 go.work：从 webook/ 用带路径的 go 命令可跨模块（workspace 生效）；
# 裸 ./... 不跨模块，需带子路径，或 cd 进各模块目录。
go build ./internal/... ./chat/...    # 编译指定模块（或 cd <svc> && go build ./...）
go vet ./internal/... ./pkg/...       # 静态分析
go test ./internal/integration/...    # 集成测试（需 MySQL + Redis；cd internal && go test ./... 亦可）
cd internal && wire ./...             # 重新生成 core 的 wire_gen.go（各模块同理 cd <svc>）
make wire                             # 一键重生成全部模块的 wire_gen.go
make -f mk/mock.mk mockgen            # 重新生成 Mock（内部逐模块 cd）
make -f mk/es.mk help                 # ES 管理命令
make -f mk/infra.mk help              # 基础设施管理

# 启动 core 示例（默认用 config/local.yaml）
cd internal && APP_ENV=config/local.yaml go run .
```

## 环境说明

应用配置命名标准（**config 层**，不同于部署层 `.env.<env>`）：

| 文件 | 角色 | 谁用 | 特征 |
|------|------|------|------|
| `config/local.yaml` | 本地开发 | 你 Windows `go run`、IDE 调试 | 明文密码连本机 docker compose |
| `config/dev.yaml` | 团队共享 dev / CI 集成测试 | 部署 `webook-dev` project | `otel.env=dev`, `sample_ratio=1.0`, logger debug |
| `config/staging.yaml` | 预发布 | 部署 `webook-staging` project | `otel.env=staging`, `sample_ratio=0.5` |
| `config/prod.yaml` | 生产 | 部署 `webook-prod` project | `otel.env=prod`, `sample_ratio=0.1`, logger info |

**配置方案**（详见 `prd/config/config-architecture.md`）：
- 5 份 yaml（local/dev/staging/prod/test）同构，域分组 `server`/`client`/`data`，叶子键 snake_case，时间值 duration 字符串
- **密钥不进 git**：外部凭据（LLM/embedding apiKey）+ 自建中间件密码全走 `${ENV}` 占位（`${MYSQL_PASS}`/`${REDIS_PASS}`/`${DEEPSEEK_API_KEY}`/`${KIMI_API_KEY}`/`${QIANFAN_API_KEY}`），viperx 加载时**解析 yaml 后在配置值上展开**（仅 `${NAME}` 形式，展开发生在已解析的字符串值上、不经 yaml 解析器 → 密钥含特殊字符也无法注入结构）；local/test 的自建中间件密码可明文
- **本地跑起来（裸 `go run` / `go test`）**：这些 `${ENV}` 外部凭据不在 yaml，需自己给值。`viperx.LoadLocal` **自读「配置文件同目录」的 `.env`**（`config/local.yaml`→`config/.env`；与 yaml 同目录、不依赖 CWD；godotenv 解析，仅填未设置的键，优先级 真实环境变量 > `.env`）——拷 `<svc>/config/.env.example` 为同目录 `.env` 填真实值即可（如 `webook/internal/config/.env`）。也可直接设 shell / IDE 环境变量。MySQL/Redis 在 local.yaml 是明文无需设。**`.env` 持密钥必须 gitignore，只 track `.env.example`**
- **读取**：各 ioc `viper.UnmarshalKey("<段>", &cfg)` 就地读并直接用，**无中央 Bootstrap、无 Validate 校验层**，缺配置在消费点自然失败（空 target→dial 失败 / 坏 dsn→连库失败）
- 段类型各归其位：grpcx 自带 `ServerConfig`/`ClientConfig`（+ 就地默认），其余叶子段在消费它的 ioc 内联定义，不建 config 包
- 调参键（ttl/timeout/kafka 超时等）yaml 显式写、可逐环境改；代码默认仅 fallback
- 全叶子键 snake_case，**无业务段例外**：`llm.providers[]` / `embedding` / `ollama` / `migrator.*` pipeline 的 config struct 已补 `mapstructure:"snake_case"` tag（与 infra 段同款），yaml 一律 snake（`api_key` / `base_url` / `max_tokens` / `batch_size` / `task_name`…）。注：viper 大小写不敏感匹配下，snake_case 带下划线必须有 tag 才绑得上
- docker 部署：`.env.<env>` 的 `APP_ENV` 决定加载哪份 yaml；`MYSQL_PASS`/`REDIS_PASS`/`DEEPSEEK_API_KEY`/`KIMI_API_KEY`/`QIANFAN_API_KEY` 经 docker-compose `environment` 转发进容器供 `${ENV}` 展开

**docker-compose 动态选 yaml**：`APP_ENV: "${APP_ENV}"`，`.env.<env>` 里填 `config/<env>.yaml`。

**L2 K8s 演进**：yaml 进 ConfigMap，密码剥到 K8s Secret，`envFrom.secretRef` 注入容器环境变量，应用侧加 `viper.BindEnv("data.mysql.dsn", "MYSQL_DSN")` 显式绑定 env key → yaml 字段（纯 AutomaticEnv 对嵌套 key 不生效已实测验证）。

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

### 多服务布局约定

- **core**（最早）：整个服务在 `internal/` 下（含 `main.go` / `wire.go` / `ioc/`），历史遗留布局
- **拆分服务**（`chat/`、`migrator/`、`interaction/`、`worker/`）：包平铺在 `<svc>/` 下，**不套 `internal/`**——单 module 仓库里 `<svc>/internal/` 只能挡仓内兄弟服务导包，当前无跨服务导包需求，边界靠 review 维持
- **新增服务跟随平铺布局**，不效仿 core；每个拆分服务各自带 `<svc>/CLAUDE.md`（拆分原因 / 边界 / 接入方）
- **端口分配（铁律，新增服务动手前必查现有占用）**：业务服务走 `80x0/80x1` 段——**每服务独占一个十位段**，HTTP（metrics/health 或 core 网关）= `80x0`、gRPC = `80x1`。已占用：core `8010/8011` · chat `8020` · comment `8030/8031` · interaction `8040/8041` · worker `8050` · relation `8060/8061` · tag `8070/8071` · search `8080/8081` → **下一个新服务用 `8090/8091`**。运维控制台（migrator）走 `82xx`、otel `88xx`、exporter `9xxx`。**禁止把新服务端口塞进别人的十位段**（如 relation 用 `8042` 撞进 interaction 的 `804x` 段）；config 5 份 yaml 的 `server.http.addr`/`server.grpc.addr` + deploy(compose/prometheus/nginx) 一致按此分配
- 接 etcd 配置热更的服务（core / chat / migrator / interaction）`ioc/config.go` 同构持有 `ConfigChangeCallbacks`（`web.go` 追加回调、`main.go` 统一触发），新服务接热更时镜像此文件；**`worker` 是例外**——纯静态本地配置（只 `LoadLocal` 不 `WatchRemote`），etcd 仅用于 gRPC 服务发现，配置变更靠重启

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
- **包内多实现的文件组织（接口 + N 实现的包）**：契约与实现分文件，杜绝"实现散落、一半挤在接口文件"——
  - `<pkg>.go`（与包同名）：**只放接口 + 共享值类型**，禁止任何具体实现（哪怕最简单的 Identity/NoOp 默认实现）
  - dispatch（factory / registry）：独立文件 `factory.go` / `registry.go`
  - 每个具体实现各占一个**按行为/技术命名**的文件（`mysql.go` / `mongo.go` / `identity.go`）
  - 范本：`pipeline/source`（source.go + factory.go + mysql/canal/es/mongo.go）、`pipeline/sink`、`pipeline/transform`

### 三层职责边界（web / service / repository）

> 一句话：**接入层只搬运、service 编排业务、repository 只存取**。跨源聚合是业务逻辑，永远在 service。
> **接入层 = `web/`（core HTTP handler）或 `grpc/`（拆分服务的 gRPC server）**——两者同一层、同等"薄"，只是协议不同（HTTP VO ↔ pb）。

| 层 | 只做 | 禁止 | 依赖 |
|----|------|------|------|
| **接入层**：`web/`(HTTP handler) / `grpc/`(gRPC server) | 参数绑定/校验（拦截器）、调 service、domain↔VO/pb 映射（`toPb`/`toDomain`/`toVO`）、错误→HTTP/gRPC status | 业务逻辑、跨源聚合编排、带业务语义的私有小方法（如 `commentCounts`/`likedTotalByAuthor`）、直接持有下游 gRPC/外部 client、循环里发 RPC | 只依赖 `service.XxxService` 接口（+ 映射用 domain/VO/pb） |
| **service**（`service/`）| 业务规则/校验、**跨源聚合**（多 repo / 其它 service / gRPC client 组合）、降级策略、事务边界决策 | 直接读写 DB/Redis（走 repo）、拼 HTTP/VO/pb | repo 接口、其它 service 接口、**gRPC client（聚合依赖放这层）**、logger |
| **repository**（`repository/`+`dao/`+`cache/`）| 持久化 + 缓存（Cache-Aside）、domain↔entity 转换 | 业务逻辑、跨服务调用、聚合别的领域 | dao、cache |

**判定法**（写一段代码前自问）：涉及「≥2 个数据来源」或「有业务含义的加工/降级」？→ service。只是「把 service 结果摆成 VO/pb」？→ 接入层（web/grpc）。只是「一张表 / 一个缓存的存取」？→ repository。

**接入层（web / grpc）出现这些即越界**（下沉 service）：`for … { cli.XxxRPC() }`；`svcA.Find…()` + `cliB.Batch…()` 拼装；`func (h *Handler/Server) xxxCounts(...)` 这类业务私有方法；handler/server 结构体持有**下游** `xxxv1.XxxServiceClient` 原始 gRPC client 做聚合。
**接入层允许的"映射"**（不算业务）：`domain(+聚合结果) → VO/pb` 纯函数（`toXxxVO`/`toPb`）、`slicex.Map`、`err → *errs.Error`/gRPC status。
> 注：gRPC server 用自己服务的 service 是本分；但**为聚合而持有别的服务的 gRPC client** 属越界（该聚合应在 service 层）。

**纯转换 vs 业务小方法**（`toPb` 为何不算被禁的小方法）：`toPb`/`toDomain`/`toVO` 是**传输格式 ↔ domain 的纯映射**（无 I/O、无跨源、无业务判断），属接入层本职——`toPb`(gRPC server) ≡ `toVO`(HTTP handler)，放 `grpc/` 或 `web/` 都对，别为它单开"转换层"。被禁的"小方法"特指 `commentCounts` 这类**会发 RPC / 做聚合 / 做降级决策**的业务逻辑。判据：**方法体里有调用或业务分支 → 下沉 service；纯搬字段 → 留接入层。** 转换按层各管一段：接入层管「传输 ↔ domain」，repository 管「domain ↔ dao」（gRPC 路径的 repo 同样只做 domain↔dao、不碰 pb）。

**gateway/BFF handler 同规**：core 作网关聚合下游（interaction/comment/relation gRPC + user 昵称）也是业务聚合——放 service，接入层只调用 + 映射。参见 `service.GRPCCommentService` / `GRPCRelationService`（持下游 gRPC client + userSvc + producer，聚合评论树/关系列表 + 昵称 + 计数 + 事件），对应 web handler 已瘦身为「调 service + `slicex.Map` 映射 VO」。

范例：`/article/reader/author` 的互动/评论/获赞聚合在 `ArticleReaderService.AuthorArticles`（该 service 持有 comment gRPC client），web `Author` 仅 `svc.AuthorArticles(...)` + `slicex.Map(items, toReaderArticleVO)`。

## 命名规范

| 维度 | 规则 | 示例 |
|------|------|------|
| 接口 | `[实体][业务角色][层]` | `ArticleAuthorDAO` |
| 实现 | `[技术限定][实体][业务角色][层]` | `GormArticleAuthorDAO` |
| 表名 | **单数**，下划线分词 | `article` / `published_article` / `user_interaction`（❌ `articles`） |
| 文件 | 按业务角色，不按技术 | `article_author.go`（非 `article_dao.go`） |
| receiver | 类型首字母小写 | `func (s *chatService) Send()` |
| 标识符缩写 | **统一 `Id` 风格**（不用 Go initialisms 的 `ID`），字段/方法/局部变量一体适用，跨层字段名必须一字不差 | `Id` / `TaskId` / `BizId` / `FindById` / `taskId`（❌ `TaskID` / `taskID`） |
| 查询方法 | `Find`(单条) `Page`(分页) `List`(列表) | `FindByXx` / `PageXxs` / `ListXxs` |
| 写入方法 | 按业务动作 | `Publish` / `Withdraw`（非 `Process`） |
| 插入或确保存在 | 统一 `Upsert` | `Upsert` / `UpsertLike` / `UpsertCollect`（❌ `FindOrCreate` / `GetOrCreate` / `EnsureXxx`——全项目 insert-or-ensure/insert-or-update 一律 `Upsert`） |
| DB 查询结果 | `xxxList` | `articleList, err := dao.ListByAuthor()` |
| 自定义切片 | `xxxs` | `ids := make([]int64, 0, len(articleList))` |
| 索引/映射 | `xxxMap` | `authorMap := make(map[int64]Author)` |
| 单条查询 | 原名或无后缀 | `article, err := dao.FindById()` |

### 缩写标准表（Go initialisms，整体大写）

缩写在标识符里**整体大写**（词首、词中都大写），不写半大写驼峰；跨层（Go 字段 ↔ pb ↔ 前端 TS）**一字不差**。来源：Go 官方 `golang/lint` initialisms。

**Go 标准缩写**（golint 全表）：
`ACL API ASCII CPU CSS DNS EOF GUID HTML HTTP HTTPS ID IP JSON LHS QPS RAM RHS RPC SLA SMTP SQL SSH TCP TLS TTL UDP UI GID UID UUID URI URL UTF8 VM XML XMPP XSRF XSS`

**项目特有缩写**（同样整体大写）：`AI ES OK SSID SMS LLM SSE VO DTO`

| ✅ 正确 | ❌ 错误 |
|--------|--------|
| `Id` `UserId` `TaskId` `BizId`（与 pb 对齐）| `ID` `UserID` `TaskID` `BizID` |
| `FindById` `GetById` | `FindByID` `GetByID` |
| `URL` `AuthURL` `CallbackURL` | `Url` `AuthUrl` |
| `HTTPClient` `HTTPServer` | `HttpClient` `HttpServer` |
| `JSONBody` `ParseJSON` `APIKey` | `JsonBody` `ParseJson` `ApiKey` |
| `SSID` `SSIDKeyPattern` | `Ssid` `SsidKeyPattern` |

**唯一小写例外**：孤立的局部变量 `id`（`id := ctx.Param("id")`）可全小写；一旦拼进复合词或作导出标识符，必须大写——`userID` / `UserID`，**不是** `userId`。

## 层间模型转换（domain ↔ dao / domain ↔ pb）

转换是**纯函数**，不是数据库操作——**禁止用查询动词（List / Find / Page）给转换方法命名**。

两条铁律（why：单条是字段映射的**唯一真相源**，加字段只改一处；批量循环是无差别样板，不值得每个 repository / gRPC server 写一份）：

1. 只写**单条**转换，方向编进名字（两侧类型常同名，按类型命名分不清方向）：
   - repository 层（domain ↔ dao）：`toDomain`（dao → domain）/ `toEntity`（domain → dao）
   - gRPC 层（domain ↔ pb）：`toPb`（domain → pb）/ `toDomain`（pb → domain，如需要）
   - **一个 repository/层持多种 domain 类型时**（如 relation 的 stats/edge/block）：方向前缀不变、加类型后缀 `toDomain<Type>` / `toEntity<Type>`（如 `toDomainStats`/`toDomainEdge`/`toDomainBlock`），仍是**接收者方法**、单条、批量走 `slicex.Map(list, r.toDomain<Type>)`。**禁止纯类型名**（`toStats`/`toEdge`）——分不清方向，违背本条 why
2. **批量一律 `slicex.Map(rows, toDomain/toEntity/toPb)`**（`pkg/slicex`），禁止手写 `toDomains`/`toXxxList`/`toXxxSlice` 等复数转换方法

```go
// repository：domain ↔ dao
func (r *X) toDomain(m dao.Y) domain.Y { return domain.Y{...} }   // 唯一映射点
list, total, err := r.dao.List(...)
return slicex.Map(list, r.toDomain), total, nil                    // 批量=泛型 Map+单条

// gRPC server：domain → pb（范本 comment/grpc、interaction/grpc）
func toPb(d domain.Y) *yv1.Y { return &yv1.Y{...} }                // 唯一映射点
return &yv1.ListResponse{Items: slicex.Map(list, toPb)}, nil       // 批量同理
```

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
10. **脚本同步**：所有表结构改动必须同步到对应 SQL 脚本（含列注释），不止改 Go struct 让 AutoMigrate 发挥；视图 `v_xxx` 也同步更新。**脚本落点按服务归属**：core（`internal/`）表 → `webook/scripts/webook.sql`；拆分服务表 → `<svc>/scripts/<svc>.sql`（如 `comment/scripts/comment.sql`、`relation/scripts/relation.sql`、`tag/scripts/tag.sql`）。纯 ES 服务（search）无 SQL 脚本，schema 真相源是 `<svc>/repository/dao/*_index.json`（见 ES 索引规范）

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

## ES 索引规范

- **索引一律「别名 + 版本化」**（业界标准）：app 与查询只认稳定的逻辑**别名**（如 `article`），物理索引是版本化的 `<别名>_v1`。`ensureIndex` 幂等确保 `别名→物理`（存量只有物理、无别名时补挂别名、数据不动）。改 mapping = 建 `<别名>_v2` → reindex → 原子切别名 → 删旧，**零停机**。禁止让 app 直接写死物理索引名。运维见 `make -f mk/es.mk help`。
- **mapping 单一真相源** = 服务内 `//go:embed` 的 json（如 `search/repository/dao/article_index.json`）；`es.mk` 直接读同一份，禁止两处内联漂移。
- **多实体检索不强行 generic**：检索的相关性/查询逐 biz 不同（article 向量 kNN、user 前缀补全、comment 全文），**通用的是基础设施（ES/embedding/脚手架）、不是契约**。加可搜 biz 走 per-biz 打字化搜索（typed RPC + 各自 mapping/query），照 `search/CLAUDE.md` 的「加一个可搜 biz 配方」。对照：comment/tag 能通用是因实体统一（`biz` 只是命名空间），search 不能是因相关性不统一。

## 数据迁移 SDK（业务侧接入）

`internal/migratorsdk/` 提供两个接口，业务 DAO 接入后可经迁移服务（webook-migrator）做双写 / 切流：

| 接口 | 用途 |
|------|------|
| `SwitchReader.ChooseSide(ctx, taskName, hashKey)` | 读路由：按 Redis `migrator:stage:{name}` + `migrator:gray:{name}` 决定走 OLD / NEW |
| `DualWriter.Write(ctx, taskName, fn(side))` | 写策略：按 stage 分阶段（SRC_FIRST 双写、DST_FIRST 严格双写、DST_ONLY 单写 NEW） |

**默认 NoOp 零开销**：`migrator.sdk.enabled` yaml flag 未设或 `false` 时 wire 注入 NoOp 实现，路径上无 Redis 调用、与原始 DAO 调用等价；启用迁移时改 yaml 切到 RedisSwitchReader/RedisDualWriter。

**migrator 服务挂掉不影响主服务**：SDK 不调 migrator gRPC；Redis 不可达自动降级 SideOld（保业务可用）。

业务侧用法（已落 `internal/repository/article_reader.go` `CacheArticleReaderRepository`）：

```go
type CacheArticleReaderRepository struct {
    oldDAO       dao.ArticleReaderDAO
    newDAO       dao.ArticleReaderDAO // 启用迁移时操作 NEW 侧表（如 published_article_v1）
    switchReader migratorsdk.SwitchReader
    dualWriter   migratorsdk.DualWriter
    taskName     string
    // ...
}

func (r *CacheArticleReaderRepository) Upsert(ctx context.Context, a domain.Article) error {
    return r.dualWriter.Write(ctx, r.taskName, func(side migratorsdk.Side) error {
        return r.daoBySide(side).Upsert(ctx, toEntity(a))
    })
}
```

未启用时 `dualWriter=NoOpDualWriter` 等价于 `r.oldDAO.Upsert(ctx, ...)` 直接调一次。读路径同理用 `switchReader.ChooseSide` 决定 OLD/NEW。

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

## Web 层规范

> 适用：本仓库**所有** Web 层（任意 module / service / 未来新增也适用）
> 原则：横切关注点（响应格式 / 错误流转 / 日志 / 分页）由 `pkg/` 统一提供，业务代码**只写业务**
> 违反任意一条 review 直接打回

### 7 条规则（每条都有 why）

| # | 规则 | Why（原理）| How（怎么做）|
|---|------|------|------|
| 1 | **响应类型用 type alias，不自造 struct** | 多个 Result struct 让前端 / 网关无法统一处理；单一定义源是 API 契约的基础 | `type Result = ginx.Result`（来源 `pkg/ginx`）|
| 2 | **路由装饰器用框架，不手写 c.JSON** | 手写 `c.JSON(400, ...)` 让错误映射分散到每个 handler；新增一种错误要改 N 处 | 只两个 wrapper：`ginx.Wrap`（无请求体）/ `ginx.WrapReq[Req]`（绑 JSON，失败自动 400 `BAD_REQUEST`）；登录态用 `ginx.MustClaims[C](ctx)`（受保护路由）/ `ginx.Claims[C](ctx)`（可选登录）从 ctx 取，无 WrapClaims 变体 |
| 3 | **分页响应用 PageResult，不自造 ListResp** | 同 #1，前端依赖统一 `{list, total}` 形态 | `ginx.PageResult{List, Total}` |
| 4 | **Handler 签名 `(ctx)` 或 `(ctx, req)` → `(Result, error)`** | 配合 #2：成功框架填 `code=200`、`msg` 空补 "OK"、`*errs.Error` 转对应 HTTP、其他 err 转 500 → `body.code ≡ HTTP status` 全链路自洽 | handler 只 `return Result{Data:x}, nil`（不写 code）；见模板段 |
| 5 | **业务错误用 sentinel `*errs.Error`，自带 HTTP code** | `errors.New("xxx")` 是 string 比较；handler 内多分支 `errors.Is` switch 不可维护 | `pkg/errs.New(httpCode, msg).WithReason(REASON)` 定义包级 sentinel；`WithCause` 附原因；`WithMetadata` 附字段定位 |
| 6 | **日志用 `LoggerX` 接口，不自造 Logger** | 框架统一接日志 / 追踪 / 采样；自造接口无法接入 OTel / Field 化结构日志 | `logger.LoggerX`（`pkg/logger`）+ Field helper（`logger.Int64 / String / Error`）+ `logger.NewNopLogger()` 测试用 |
| 7 | **双级错误标识：`code`=HTTP 传输层粗分类，`reason`=业务身份** | HTTP code 空间小、一码背多类业务错误（限流全是 429）→ 监控只能 `by status` 看不出哪类业务、前端只能猜 `msg`；`reason`（稳定枚举）补足业务维度。详见 `prd/error-model` | `code`/`reason`/`message` 同在 `*errs.Error` 单一源（`.WithReason`），`code`≡`Result.Code`≡HTTP status；**禁止**自造平行 `ErrCodeXxx string` 常量体系（双重契约） |

### 速查 ❌ / ✅

| 禁止 | 必须 |
|------|------|
| 自定义 `Result struct {...}` | `type Result = ginx.Result` |
| 业务包里手写 `code` 字面量 / 自造 Result helper | 用 ginx 命名构造器 `ginx.OK(data)` / `OKWith` / `NotFound` / `BadRequest` / ...（名字带 code）；登录态 `MustClaims / Claims` |
| 自造 `XxxListResp / PageResp` 分页结构 | `ginx.PageResult{List, Total}` |
| `func(c *gin.Context)` + `c.JSON` + switch 错误映射 | `func(ctx) (Result, error)`，框架 `WriteError` 自动转 |
| `errors.New("xxx")` 当业务错误 | `pkg/errs.New(httpCode, msg)` 定义 `*errs.Error` |
| 自造 `Logger interface` / `NoOpLogger` | `logger.LoggerX` + `logger.NewNopLogger()` |
| 平行 `ErrCodeXxx string` 错误码常量体系（双重契约） | `code`=HTTP status + `reason`=业务原因码，同在 `errs.Error`（`.WithReason`） |

### Handler 标准模板

```go
// 接口 + Internal 实现 + 构造函数返回接口
type XxxHandler interface {
    RegisterRoutes(r *gin.Engine)
}
type InternalXxxHandler struct {
    svc service.XxxService
    l   logger.LoggerX
}
func NewXxxHandler(s service.XxxService, l logger.LoggerX) XxxHandler {
    return &InternalXxxHandler{svc: s, l: l}
}

// 业务方法签名固定 (ctx) (Result, error)
func (h *InternalXxxHandler) Create(ctx *gin.Context, req createReq) (Result, error) {
    id, err := h.svc.Create(ctx.Request.Context(), ...)
    if err != nil {
        return Result{}, err          // *errs.Error 自动转 HTTP；其他 err → 500
    }
    return Result{Data: gin.H{"id": id}}, nil
}

// 路由装饰
func (h *InternalXxxHandler) RegisterRoutes(r *gin.Engine) {
    g := r.Group("/xxx")   // ⚠ 不带 /api 前缀！见下「路由前缀铁律」
    g.POST("", ginx.WrapReq[createReq](h.Create))
    g.GET("/:id", ginx.Wrap(h.Get))
    g.GET("", ginx.Wrap(h.List))  // List 返回 Result{Data: ginx.PageResult{...}}
}
```

**需登录的 handler**：签名不变（`(ctx)` 或 `(ctx, req)`），体内取登录态——
```go
func (h *InternalXxxHandler) Mine(ctx *gin.Context) (Result, error) {
    uc := ginx.MustClaims[UserClaims](ctx)  // 受保护路由：鉴权中间件已保证，缺失即 panic→500（漏挂中间件的 bug）
    // 可选登录路由用：uc, ok := ginx.Claims[UserClaims](ctx)
    ...
}
```
各服务启动时 `ginx.UserKey = consts.UserKey` 设一次（见 `ioc/web.go`），`MustClaims/Claims` 据此从 ctx 取。

**快速构造响应**：`ginx.OK(data)` / `OKWith(msg, data)` / `NotFound(msg)` / `BadRequest(msg)` / `Conflict(msg)` / ...（名字带 code，只给 msg/data，wrapper 按 `Result.Code` 作 HTTP status）。用法 `return ginx.NotFound("文章不存在"), nil`。这些无 reason，需按 reason 监控/前端分支的业务错误仍返回 `errs.ErrXxx` sentinel（带 reason + 日志）。

### 路由前缀铁律（core 路由禁带 `/api`）

**core 的 HTTP 路由组一律不带 `/api` 前缀**，与现有 `/interaction`、`/article`、`/user`、`/comment` 同形。

- 前端 axios `baseURL = '/api'`，浏览器请求 `/api/xxx/*`；dev server rewrite(`webook-fe/next.config.ts` `/api/:path*`→core) 与生产 nginx(`deploy/nginx/conf.d/default.conf`) **都会剥掉 `/api`** 再转给 core。
- 所以 core 注册 `Group("/api/xxx")` = 实际暴露 `/api/xxx`，而请求剥前缀后是 `/xxx` → **404**。注册 `Group("/xxx")` 才对。
- 中间件 `IgnoredPaths`/`OptionalPaths` 同理写**剥后路径**（`/xxx/list`，不是 `/api/xxx/list`）。
- 例外：拆分服务有自己的 rewrite 段（如 chat `/api/chat/:path*`→chat `/chat/*`），其路由在 `/chat/*` 下。新增直连前端的服务必须同步加 next.config + nginx 的 rewrite 段。
- 新增 core 路由前先 `grep -rn 'Group("' internal/web/` 对齐，禁止照搬带 `/api` 的旧模板。

### 业务错误定义模板

```go
package errs

import "github.com/boyxs/train-go/webook/pkg/errs"

// 包级 sentinel：每个必须 .WithReason(SCREAMING_SNAKE 业务原因码，全局唯一)——过 TestAllSentinelsHaveReason guard。
// Is 优先按 reason 比对（Message 改文案不破坏匹配 / 不再要求全局唯一）。
var (
    ErrResourceNotFound  = errs.New(404, "资源不存在").WithReason("RESOURCE_NOT_FOUND")
    ErrDuplicateResource = errs.New(409, "资源已存在").WithReason("RESOURCE_DUPLICATE")
    ErrInvalidArg        = errs.New(400, "参数不合法").WithReason("ARGUMENT_INVALID")
    ErrForbidden         = errs.New(403, "无权访问").WithReason("FORBIDDEN")
    ErrUnauthenticated   = errs.New(401, "未登录").WithReason("UNAUTHENTICATED")
)
```

### 业务错误用法

```go
// 抛出（handler 直接返回，框架自动转 HTTP）
return Result{}, errs.ErrResourceNotFound

// 带原因（cause 进日志 + e.Error() 输出，不影响 errors.Is 命中）
return Result{}, errs.ErrInvalidArg.WithCause(fmt.Errorf("name 不能为空"))

// 带元数据（前端可用 Result.Metadata 做字段级精确提示）
return Result{}, errs.ErrInvalidArg.WithMetadata("field", "name")

// 调用方匹配（优先按 reason 比对，跨副本 / 跨 gRPC 重建实例仍命中）
if errors.Is(err, errs.ErrResourceNotFound) { ... }
```

### Web 层 PR 自查清单

- [ ] 响应类型用 `type Result = ginx.Result`，没有自造 Result struct
- [ ] 路由只用 `ginx.Wrap / WrapReq` 装饰（登录态用 `MustClaims / Claims` 从 ctx 取），没有 `Result400 / 422 / 500 / OK` 等响应 helper
- [ ] 分页用 `ginx.PageResult`，没有自造 `XxxListResp / PageResp`
- [ ] Handler 签名 `func(ctx)` 或 `func(ctx, req)` → `(Result, error)`；成功 `return Result{Data:x}, nil` 不手写 code（框架填 200）
- [ ] Handler 内**无** `errors.Is` 多分支 switch 映射 HTTP code（让 `*errs.Error` 自带 Code）
- [ ] 业务错误用 `pkg/errs.New(httpCode, msg)` 定义 sentinel，不用 `errors.New`
- [ ] 日志用 `pkg/logger.LoggerX` + Field helper，没有自造 Logger interface / NoOpLogger
- [ ] 业务 sentinel 都带 `.WithReason(SCREAMING_SNAKE)`（过 `TestAllSentinelsHaveReason` guard）；不自造平行 `ErrCodeXxx string` 常量体系
- [ ] 三层（Handler / Service / Repository）的构造函数都**返回接口**，实现命名带 `[技术 / Internal 前缀][实体名][层后缀]`（如 `InternalResourceService`、`GormResourceDAO`）

## 编码约束

- 日志用注入的 `LoggerX`，禁止 `fmt.Println`
- 错误必须处理，禁止 `_ = err`
- 测试假数据用 `// ===== TODO: 测试假数据 START/END =====` 包裹
- 大数据量查询优先在数据库内完成过滤和聚合，避免全量加载到内存

## 集成测试规范

拆分服务的集成测试统一布局（core `internal/` 同理），两条铁律：**setup 用 wire 装配**、**integration 放 `main.go` 锚文件**。

- **`<svc>/integration/`**：测试本体，包名 `package integration`（内部测试包）。该目录**只有 `_test.go`** —— 是个 test-only 目录。
- **`<svc>/integration/setup/`**：测试依赖装配。**必须用 wire**（`wire.go` injector + 生成的 `wire_gen.go`）产出测试要的 handler/service/DAO/server；**禁止在 setup 或各测试 SetupSuite 手写 `NewXxx(...)` 装配链**（那样加依赖要改一堆测试、与生产 wire 图脱节）。真实中间件的 `InitDB`/`InitRedis`/`InitESClient` 等放 setup 的非测试文件（`db.go`/`redis.go`/`es.go`）并进 provider set。范本 `internal`/`comment`/`interaction` 的 `integration/setup/wire.go`。
- **跨服务依赖用 fake 桩**：集成测试不拉起兄弟服务的 gRPC server / etcd，用 non-tagged `fake_*.go` 定义 no-op 桩（返零值/空）在 setup 的 provider set 注入。范本 `internal/integration/setup/fake_interaction.go`（`FakeInteractionService`）、`fake_search.go`（`FakeSearchService`/`FakeTagService`）。端到端测某服务本身走它自己的 `integration`（真依赖 / bufconn）。
- **运行**：需对应真实中间件（MySQL/Redis，按服务另加 ES/Kafka/Redis cluster）。`cd <svc> && go test ./integration/...`。地址/测试库由各服务 `config/test.yaml` 指定——注意有的服务 test.yaml 连 Redis **cluster**（`7001-7003`，验 CROSSSLOT），本地需起对应集群，否则每次 Redis 操作重试超时、整套跑不完。

### main.go 锚文件（铁律）

**每个 test-only 的 `integration/` 目录必须放一个非测试文件 `main.go`**（`package integration` + 包 doc 注释），否则 `wire ./...` / `make wire` 会失败。用 `main.go`（不用 `doc.go`）：集成测试必有 `main_test.go`（TestMain），`main.go` 与之成对、一眼可辨是该包的非测试主文件。

- **为什么**：`wire ./...` 加载模块下**所有**包（含 test-only 的 `integration`）。wire 的 `Generate` 主循环对每个包**先**调 `detectOutputDir(pkg.GoFiles)` 推导输出目录、**再**检测 injector。test-only 目录的 `pkg.GoFiles`（非测试 `.go`）为空 → `detectOutputDir` 报 `"no files to derive output directory from"` → 该包记错 → 整条 `wire ./...` 退 1（**与有没有 injector 无关**，错在 injector 检测之前）。
- **`main.go` 怎么救**：给该目录一个非测试文件 → `pkg.GoFiles` 非空 → `detectOutputDir` 通过 → wire 发现该包无 injector → 干净跳过（`Content` 为空、不写文件）。于是 `wire ./...` 正常退 0。
- **禁止用 Makefile 兜底容错**（吞退出码 / grep 出 injector 目录再逐个 `wire gen`）——那是兼容 wire 的坑，不是修坑。把锚文件放对位置，让惯用的 `wire ./...` 本身就绿。
- 范本 `webook/tag/integration/main.go`。新建**任何** test-only 包（不限 integration）同理需锚文件。

## 提交前必跑 goimports

CI 用 `goimports -local github.com/boyxs/train-go/webook -l ./<svc> ./api ./pkg` 检查 import 顺序，乱了直接 fail。**任何 commit 前必须在 `webook/` 跑一次原生命令**（`-local` 用统一前缀即可覆盖全部子模块；从 webook/ 递归格式化跨模块生效）：

```bash
cd webook && goimports -local github.com/boyxs/train-go/webook -w .
```

把本地 import 自动分组到第三组，避免 CI 红 + 二次 commit。`workflow:done` 流水 build/lint 之前必须先跑这条命令再继续验证。

**禁止套 `make fmt`**——直接调底层二进制，规则透明可审计，不依赖 Makefile 黑盒。
