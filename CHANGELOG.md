# CHANGELOG

<!-- 新功能前插在此，日期降序 -->

## [2026-07-22] logger 强制 ctx 重构 + gRPC 日志字段命名空间

**变更内容**: `pkg/logger.LoggerX` 从"可选 ctx"（`WithContext(ctx)` 派生 + 4 个无 ctx 基础方法）**破坏性收敛为强制 ctx**：接口只剩 `Debug/Info/Warn/Error(ctx, msg, ...Field)` 4 方法，删除 `WithContext` / `XxxContext` / `ZapLogger.ctx` 字段。全仓 **213 处调用点一次性迁移**（`X.WithContext(ctx).Xxx()` → `X.Xxx(ctx, ...)`；裸调用补 ctx，真无 ctx 处显式 `context.Background()`）。顺带把 gRPC logging 拦截器 access 字段**全部**加 `grpc.` 命名空间（`grpc.cost/type/method/event/peer/peer_ip/panic/stack`，对齐已有 `grpc.code/message`）。
**影响范围**: `pkg/logger/{types,zap_logger,nop}.go` + 测试；10/12 模块有调用点（pkg·internal·chat·comment·interaction·migrator·relation·search·tag·worker，~80 文件）；`pkg/grpcx/interceptor/logging/builder.go` + 测试（字段命名空间）；cronx/gormx/grpcx 的 LoggerX 测试替身（签名 + 删 WithContext）。
**技术决策**: ① **强制 ctx 而非保留可选**——上一版（[2026-07-21]）走增量非破坏 `WithContext` 是为逐服务灰度；灰度完成后本次全量收敛，杜绝"忘带 ctx 丢 trace"，方法名不带后缀（`Debug(ctx,...)` 对齐标准库 slog 的 ctx-first 心智，而非 `DebugContext` 后缀派）；② Go 无方法重载 → 同名新旧签名无法并存，破坏性变更必须一次性全改，用 workflow 并行迁移（pkg 先编译绿再 fan out 9 个 service，各 agent `go build && go vet` 自验）；③ 真无 ctx 场景（ioc/main/后台 goroutine/panic handler/构造函数）显式 `context.Background()`（可 grep 审计"无 trace"日志点），有 ctx 处一律传（kafka `ExtractTraceContext(msg)`、异步走 goSafe 的 `bgCtx`、HTTP audit `c.Request.Context()`、migratorsdk 把原丢弃的 `_ ctx` 接回）；④ 删 `WithContext` 一并消除其返回浅拷贝的实例分配；⑤ gRPC 字段 `grpc.` 命名空间——文件本有 `grpc.code/message`，补齐其余避免顶层裸 `method`（gRPC 全限定名）语义不自明 + 与 HTTP `request.*` 潜在撞名。
**待办**: 无（gRPC 日志字段已全部收敛到 `grpc.*`；Kibana 侧无遗留旧字段查询需迁移）。
**会话**: 260722-logger-强制ctx重构
**发布**: 待上线（应用侧改动，随下次出镜像生效）

## [2026-07-22] ELK 日志运维硬化 + gormx 标准化

**变更内容**: Phase 1 起栈实跑后按问题硬化。**日志降噪**：Filebeat 从"通配所有 `webook-*`"改为**只采 9 个 Go 业务服务**（中间件/nginx/fe/ELK 自身不采，砍 ~87% 非应用日志）；Logstash 丢非 JSON、级别交各服务 `logger.level` 控（移除管道内 debug 硬过滤——prod=info 本就不产 debug，管道保持环境通用）；gRPC 拦截器正常 RPC Info→Debug、错误/panic→Error；`pkg/logger.NewZapLogger` 加 `AddCallerSkip(1)` 让 `log.origin` 指真实业务行。**修复**：Kibana 内存 768m→1024m（低于 1G 启动即 Node JS heap OOM）；`error` 定为标量（zap StacktraceKey `error.stack_trace`→`stack_trace`，避免与标量 `error` 撞成对象被 ES 拒 400）；日志索引 `webook-logs-*`→`logs-webook-*`（进 `logs-*` 命名空间，Kibana「All logs」默认 data view 直接命中）+ 模板 `priority:200` 盖 ES 内置 `logs` data-stream 模板；`webook-es-setup` 一次性 init 容器起栈自动建 ILM/模板 + 设 kibana_system 密码（根治 Kibana 启动撞车）。**gormx 标准化**：`GormLogger` 对齐 `gorm.io/gorm/logger.Config`（`Config{LogLevel/SlowThreshold/IgnoreRecordNotFoundError}`，单构造 `NewGormLogger(l, cfg...)` 通吃默认/自定义），Trace 按严重度映射 app 级——正常→Debug、**慢查询(≥200ms)→Warn**（prod info 也可见）、真错误→Error。**Logstash→ES TLS 脚手架**：`LOGSTASH_ES_SCHEME/SSL_ENABLED/CA_FILE` env 驱动（默认 off，staging/prod 建议开）。**deploy.sh**：service 参数支持正则匹配展开。
**影响范围**: `deploy/elk/filebeat/filebeat.yml`（白名单）· `deploy/elk/logstash/pipeline/webook.conf`（降噪/时间戳/TLS）· `deploy/elk/es/setup.sh`（logs-webook 模板 + error/stack_trace + attr:flattened）· `deploy/docker-compose.yaml`（Kibana 内存 + init 容器 + Logstash TLS 透传）· `deploy/grafana/provisioning/datasources/elasticsearch.yml`（索引名）· `deploy/deploy.sh`（服务正则）· `deploy/.env.{local,dev,staging,prod}{,.example}`（KIBANA_PORT + 资源 + `LOGSTASH_ES_*`，实际 .env 已同步）· `webook/pkg/gormx/logger.go`（标准化 + 测试）· `webook/pkg/grpcx/interceptor/logging/builder.go`（降级 + `grpc.*` 字段）· `webook/pkg/logger/{zap_config,zap_logger}.go`（`stack_trace` + AddCallerSkip）· `prd/observability/{PRD,ARCHITECTURE}.md`（Filebeat 采集范围修正）。
**技术决策**: ① **级别源头控**——ELK 管道不按级别过滤（prod=info 不产 debug），dev verbose 有意，管道跨环境通用；② **Filebeat 只采业务服务**——中间件/ELK 自身日志非应用 ECS、量大且无 trace（代价：加业务服务要更新白名单，PRD 已标注）；③ **error 标量 + stack_trace 独立**——不走 ECS `error` 对象，规避标量/对象撞车（用户定）；④ **gormx 对齐 GORM 标准 Config**——慢查询升 Warn 让 prod 可见慢 SQL、正常 SQL 仍 Debug 不刷屏；⑤ **TLS 脚手架默认 off**——CA 行默认注释（空路径会让 Logstash 启动 `:path` 校验失败），开 TLS 按 webook.conf 头部三步走。
**待办**: ① ES 角色隔离（`logstash_writer`/`kibana_reader`）仍未落地（现用 elastic 超管），prod 硬化前补；② gormx 慢查询阈值可接 yaml 逐服务调（扩展点 `NewGormLogger(l, Config{...})` 已开）。
**会话**: 260722-ELK运维硬化+gormx标准化
**发布**: 待上线（配置改动：重建 Filebeat/Logstash/Kibana + 重出应用镜像生效）

## [2026-07-21] 可观测性：ELK 日志方案设计 + 日志-链路(trace_id)接入 Phase 0

**变更内容**: 走 workflow architect 产出 `prd/observability/`（PRD + ARCHITECTURE，含 ELK 选型对比 / trace_id 必要性专章 / 分阶段落地）；随后落地 **Phase 0 应用侧日志标准化**：`pkg/logger.LoggerX` 新增 `WithContext(ctx) LoggerX`（返回浅拷贝、从 ctx 注入 `trace.id`/`span.id`，5 单测 RED→GREEN）；9 服务 `ioc/logger.go` 收敛到 `pkg/logger.InitZap()`（统一 ECS 键 + epoch_millis + `service.name/version/environment`）；access log 提 Info + trace_id（core/chat/migrator）；gRPC logging 拦截器接线全部 6 gRPC server + core；修 3 处 `zap.L()` 绕过；9 服务请求路径业务日志迁移 `WithContext(ctx)`。**Phase 1 ELK 部署（deploy 层）**：docker-compose 加 Filebeat→Logstash→Kibana 三服务（opt-in `profiles: [elk]`，复用 webook-es 存日志）+ Filebeat/Logstash 配置 + ES ILM/索引模板 `setup.sh`（幂等）+ Grafana ES 数据源；`docker compose config` + YAML/bash 语法校验通过。本地起栈调试修了 3 处 config bug（logstash `http.host`→`api.http.host` / kibana `elastic`→`kibana_system`(密码由 setup.sh 设) / filebeat command 多写 `filebeat`）；并**补齐日志 trace_id 覆盖**：GORM SQL 日志改用 `pkg/gormx.GormLogger`（实现 GORM logger.Interface + `WithContext(ctx)`，7 服务 db.go 换掉旧的无-ctx Writer 桥接、字段结构化 sql/rows/elapsed）、saramax 消费者日志用传播 ctx（handleOne/handleBatch 用 span ctx、反序列化失败从消息 headers `ExtractTraceContext`）——**请求路径 + Kafka 全链路日志均带 trace_id**，仅纯后台/启动日志无（正确）。本地实跑打通：应用→Filebeat→Logstash→ES→Kibana，Discover 按 trace.id 可关联。
**影响范围**: `prd/observability/`（PRD.md · ARCHITECTURE.md）；`pkg/logger/`（types/zap_logger/nop/zap_config + zap_logger_test）；`pkg/grpcx/interceptor/logging`；9 服务 `ioc/logger.go` + 6 gRPC `ioc/grpc.go`；core `internal/config/{prod,staging}.yaml`（body 脱敏）；tag/relation/interaction/comment/search/chat + core 的 web/service/repository/cache 业务日志调用点（~90 处）；`deploy/elk/*`（filebeat/logstash/es setup.sh）· `deploy/docker-compose.yaml`（3 ELK 服务 profile elk + grafana ES_PASS + filebeat-data 卷）· `deploy/.env.{local,dev,staging,prod}{,.example}`（8 份同步：ELK 资源 + ES 堆上调）· `deploy/docker-compose.local.yaml`（Kibana 仅本地暴露 `KIBANA_HOST_PORT`；dev/prod 服务器不裸暴露数据面 UI，走内网/socat）· `deploy/grafana/provisioning/datasources/elasticsearch.yml`。
**技术决策**: ① 采集栈选**标准 ELK**（用户定），app 侧与采集栈解耦——本期只做「日志可 trace 关联 + ECS ready」，ELK 部署(Filebeat/Logstash/ES/Kibana)列 Phase 1；② `WithContext` **增量非破坏**（保留旧 4 方法，其它模块零影响，可逐服务灰度）而非改签名（Go 无重载会让 9 模块同时炸、无法灰度）；③ **返回浅拷贝**而非就地改 `z.ctx`——logger 是注入的共享单例，就地改会并发 data race + trace 串号（语义同 zap `.With()`）；④ trace 注入用 `z.ctx==nil` / `SpanContext.IsValid()` 守卫，无 span 全场景安全 no-op；⑤ **堵住 prod/staging access log 明文密码泄漏**：提 Info 后 `allow_req_body` true→false（原 Debug 级不输出故未暴露，提级前必须关）；⑥ 背景态日志（goSafe 后台任务 / ranking 重算 / boost 协程 / producer 连接生命周期 / cmd CLI / ensureIndex 构造）保持 base logger——无请求 span，`WithContext` 是无效 no-op；⑦ **gormx GormLogger 真正 DB 错误升 Error 级**（ship review 修正）：原统一 Debug 在 prod(info) 会静默 SQL 失败，改为 `err!=nil && !errors.Is(err, gorm.ErrRecordNotFound)` 升 Error（ErrRecordNotFound / 正常 SQL 仍 Debug），确保 DB 失败在 prod/ELK 可见（补 `pkg/gormx/logger_test.go` 覆盖级别映射）；⑧ **logstash `@timestamp` 抗撞车**（ship review 修正）：app 侧数字型 epoch 直接进 Logstash 保留 `@timestamp` 会 `_dateparsefailure` / 丢真实时间，改为 grok 抽到 `[@metadata][ts_ms]` 再 `date` 覆盖（抽不到则 fail-safe 保持原值）；filebeat 补 healthcheck + `depends_on: service_healthy`，setup.sh 补 Kibana 启动顺序铁律说明；⑨ **ELK 起栈自动化 + Kibana 可达 + 索引进 `logs-*` 命名空间**：新增一次性 init 容器 `webook-es-setup`（复用 ES 镜像，等 webook-es healthy 后跑 setup.sh 即退），Kibana/Logstash 改 `depends_on: {condition: service_completed_successfully}` → 起栈自动建 ILM/模板 + 设 kibana_system 密码，根治 Kibana 启动撞车、免手动跑 setup.sh；Kibana 端口移到 base compose 发布 `KIBANA_PORT`（dev 5601 / staging 5611 / prod 5621，对齐 Grafana/Prometheus/Zipkin），删 local override 重复绑定；日志索引 `webook-logs-*` → `logs-webook-*`（进 `logs-*` 命名空间，Kibana「All logs」默认 data view `logs-*-*,logs-*,filebeat-*` 直接命中），索引模板加 `priority:200` 盖过 ES 内置 `logs` data-stream 模板（让 `logs-webook-*` 落普通按天索引而非 data stream）；⑩ **error 字段定为标量字符串**（ship 调试修正）：`logger.Error(err)` 经 zap 写标量 `error`（`err.Error()` 串），与 zap 原 StacktraceKey `error.stack_trace`（点号→把 error 变对象）在同条 error 日志里撞 → ES 400 丢整条文档。选「保 error 语义」而非 ECS `error.message`（用户定）：StacktraceKey 改独立顶层 `stack_trace`，ES 模板显式 `error`/`stack_trace` 均映射 text，`error` 恒标量不再撞（`fields.go` 键不动，仅移堆栈键）。
**待办**: ① **ELK 运行时验证**（`COMPOSE_PROFILES=elk ./deploy.sh dev` 起栈 → `webook-es-setup` 自动建 ILM/模板 + 设 kibana_system 密码 → Kibana 用默认 All logs data view（`logs-*` 已含 `logs-webook-*`，无需手建）→ 造一次跨服务请求验 trace.id 关联 + 验 `@timestamp` 为日志真实时间而非入库时间 + 确认索引落 `logs-webook-YYYY.MM.dd` 普通索引（非 data stream）——本次改动 webook-es 处 paused 未能实跑，需起栈实测）；② Kibana `trace.id` Url formatter 跳 Zipkin + Grafana 日志告警规则（PRD §12）；③ ES 角色隔离硬化（现 Logstash/Kibana 用 elastic 超管，prod 建 logstash_writer/kibana_reader）；④ Logstash Prometheus 指标需 logstash-exporter（当前仅 docker healthcheck）；⑤ gRPC client 侧 logging 拦截器（本期只接 server 侧）可选补。
**会话**: 260721-observability-ELK方案+日志trace接入
**发布**: 待上线（app 侧已落地，ELK 部署未上）

## [2026-07-21] Feed 关注流：需求 PRD + 原型 5 屏 + 架构设计

**变更内容**: 走 workflow design→architect 双阶段：广场页 `/feed` 升级双 Tab（关注 | 发现），关注流游标 + 无限滚动、卡片带互动计数 + 标签。产出 PRD（14 用户故事 / 9 条验收 / HTTP+gRPC 契约）+ Pencil 原型 5 屏（桌面 / 空态 / 未登录 / 新内容提示 / 移动端，样式复制 article 广场页 + tag chip 规范）+ 架构文档（含前瞻章节）。纯设计交付，未写代码。
**影响范围**: `prd/feed/`（prd.md · ARCHITECTURE.md · feed.pen · prototypes/01~05-*.png）；`webook/CLAUDE.md` 端口铁律行加设计预定标注（permission 8090/8091 · feed 8100/8101 → 下一个 8110/8111）。
**技术决策**: ① 独立 `webook/feed/` 纯同步 gRPC 服务（**8100/8101**，8090 已被 permission 设计先占），Kafka 消费按 worker 铁律统一收拢 worker、经 gRPC 派发回 feed；② feed 无 MySQL——收件箱/发件箱全 Redis 投影（inbox ZSET cap 2000 TTL 7d + built 标记 / outbox cap 100 TTL 1h cache-aside / bigv SET），源数据经 core article gRPC 新增 `ListAuthorArticles` 拉取，不直查他服务库；③ 推拉结合：粉丝数 ≥1000（yaml 可调）不写扩散，读时归并大 V outbox；④ 关系变更=失效重建（follow/unfollow/block 一律 DEL 收件箱下次读重建，拉黑级联天然正确）；⑤ 撤回=读时过滤（BFF FindByIds 查线上库自然滤掉）+ DEL outbox；⑥ 新增 topic `article_events`（key=authorId，core 产、失败降级记日志），ZADD/DEL 幂等 → Kafka 整批重投安全；⑦ outbox「存在才追加」需 Lua 原子，防假全量缓存。
**待办**: ① tag `TagsByBiz` 批量能力确认（否则 BFF N+1，需补批量 rpc）；② article.proto 现有 message 复用确认；③ Kafka 新 topic 建法对齐部署；④ 用户复核后进 `workflow:tdd`（22 任务 6 阶段已拆）；⑤ P1 新内容提示条（`/feed/new-count`）随 P0 后接。
**会话**: 260721-feed-关注流设计
**发布**: 待上线（纯设计文档）

## [2026-07-17] SaaS 权限系统（permission）双面完整版：需求 PRD + 原型 10 屏 + 架构设计

**变更内容**: 走 workflow design→architect 双阶段并按用户确认扩展为**双面完整版**（对标若依-pro/芋道多租户版）：租户侧（组织/成员/部门/角色/功能矩阵/数据权限/审计）+ 平台运营侧（租户管理/租户套餐/菜单管理/平台审计）。产出需求 PRD（17 用户故事 / 29 HTTP 接口 / 12 表模型 / 12 条验收标准）+ Pencil 原型 10 屏（租户侧 6+部门 1+平台侧 3）+ 架构文档。纯设计交付，未写代码。
**影响范围**: `prd/permission/`（PRD.md · ARCHITECTURE.md · permission.pen · prototypes/01~10-*.png）；其他文件零改动。
**技术决策**: ① 独立 `webook/permission/` gRPC 服务（8090/8091，对齐 tag/search 拆分范式），core 作 HTTP BFF；② 自研 RBAC1+套餐 表驱动 12 表，不引 Casbin；③ **平台侧=系统租户（org.id=1）+ Platform 套餐**，与租户侧共用同一套 RBAC 零特殊分支；④ **有效权限 = ∪(角色) ∩ 套餐**，降级裁剪不清数据（芋道语义）；⑤ 菜单树「代码管语义 / DB 管展示」：dir/menu 可 UI 全管、button 走代码注册；⑥ 数据权限 5 档（若依枚举）多角色取最宽，本期只供数不消费；⑦ 缓存三级失效：单人/角色级精确 DEL + org/全局**版本号 INCR O(1)**（`perm:over:{org}`/`perm:gver` 拼进 set key）；⑧ 鉴权 fail-closed，∩ 套餐在写缓存前完成、热路径零额外计算。
**待办**: ① **`permission.pen` 需在 Pencil 应用中手动 Ctrl+S 落盘**（MCP 改动不置应用 dirty 位、合成键被拦截，PNG/文档已全部落盘不受影响）；② 用户复核后进 `workflow:tdd`；③ 邀请触达通道（邮件 vs 复制链接）待定；④ Pencil MCP 四个坑已记录 PRD §11。
**会话**: 260717-permission-SaaS权限系统设计
**发布**: 待上线（纯设计文档）

## [2026-07-12] tag 服务 SQL 脚本补全 + 脚本落点规范对齐 + tag 服务文档

**变更内容**: 补 tag 服务此前缺失的建表脚本（靠 AutoMigrate、无脚本），并把「脚本同步」规范对齐到实际的**按服务落点**约定，补齐 tag 服务 CLAUDE.md。
**影响范围**:
- **新增 `tag/scripts/tag.sql`**：`tag`（本体）/ `tagging`（多态关联）/ `tag_follow`（关注边）三表 DDL，逐字段/类型/默认值/索引/唯一约束**严格对齐** `repository/dao` 的 GORM struct（含本轮新增的 `tag.follow_count` 列 + `tag_follow` 表），镜像 `relation/scripts/relation.sql`（表/列注释、bigint 毫秒时间、`{table}` 前缀索引名、utf8mb4_0900_ai_ci、硬删无 `deleted_at`）。throwaway DB 执行校验通过（3 表 + 索引正确）。
- **规则对齐**（`webook/CLAUDE.md` 数据表规范 #10）：由「同步到 `webook/scripts/webook.sql`」订正为**按服务归属**——core → `webook/scripts/webook.sql`；拆分服务 → `<svc>/scripts/<svc>.sql`（与既有 comment/interaction/migrator/relation 一致）；纯 ES 服务（search）无 SQL、schema 真相源是 `*_index.json`。
- **新增 `tag/CLAUDE.md`**：补齐拆分服务缺失的服务文档（为什么拆 / 边界 / 接入方 / 关键决策 / 分层 / 部署），镜像 `relation/CLAUDE.md`；部署段（prometheus job / grafana 告警 / CI / Dockerfile / compose / `.env`）经实存校验，无虚假声明。
**技术决策**: ① SQL 脚本是「AutoMigrate 之外的真相源」，与 model 对齐但不强求与 AutoMigrate 输出逐字节一致（`uint8`→`tinyint` 沿用 relation.sql 惯例）；② search 无 SQL 脚本是对的（ES 服务），规则显式说明避免将来误补；③ 三表硬删风格（tag/tagging 物理删、tag_follow status 翻转），故均无 `deleted_at`，与 relation/interaction 计数表一致。
**待办**: 无。
**会话**: 补全 tag SQL 脚本 + 文档对齐
**发布**: 待上线（纯脚本/文档；tag 表仍由 AutoMigrate 建，脚本供人工/CI/DBA 参考审阅）

## [2026-07-11] tag 详情缓存（Cache-Aside，把 Redis 引入 tag 服务）

**变更内容**: 承 architect 设计（`prd/tag/ARCHITECTURE.md §F5`），给 tag 服务**详情热点读**加 Cache-Aside，把 Redis 引入原 MySQL-only 的 tag 服务。镜像 relation 的 `RedisRelationCache` + `pkg/redisx` + `consts/cache.go` 三件套。
**影响范围**:
- **cache 层**：新 `tag/consts/cache.go`（`TagDetailPattern`=`tag:detail:%s`、`TagDetailTTL`=10min）+ `tag/repository/cache/tag.go`（`TagCache` 接口 + `RedisTagCache`：`GetDetail`/`SetDetail`/`DelDetail`，JSON 值 + jitter(0~5min) TTL）+ `tag/ioc/redis.go`（`redisx.NewClient` + prometheus hook + otel，镜像 relation）。
- **repo 织入**（`repository/tag.go`，`InternalTagRepository` +`cache`+`logger`）：`Detail` Cache-Aside（命中即返 / miss(`redis.Nil`)/故障回源 DB / 回填，失败记日志不阻断，**对外签名行为不变**）；`Follow`/`Unfollow` 真翻转时 `DelDetail(slug)`；`SyncBiz` 用 dao 新返回的 affected tagIds（新增 ∪ 删除）`FindByIds`→slugs→`DelDetail` **精确失效**。
- **dao**（`dao/tagging.go`）：`SyncByBiz` 签名 +返回 `affectedTagIds`（ref_count 变化的标签，供上层失效）。
- **配置/依赖**：5 config + test.yaml 加 `data.redis`（local/test 明文 6379、dev/staging/prod `${REDIS_PASS}`）；`wire.go` + `integration/setup/wire.go`(+`setup/redis.go`+`logger.go`) 加 Redis/cache/logger provider 重生成；**go-redis 钉 v9.18.0 对齐 relation/core**（`go mod tidy` 漂到 v9.21 会破 core `redismocks`）。
- **部署**：`docker-compose` `webook-tag` +`REDIS_PASS` env + `depends_on: webook-redis{healthy}`。
- **测试**：tag e2e +`TestDetail_CacheAside`（命中返旧值不查库 + Follow 写失效回源读新值）；`TestDetail_WeeklyNewCount` 加 `flushDetail` 保窗口测试真实性；`reset()` 清 `tag:detail:*` 保隔离。
**技术决策**: ① 只缓存 Detail（`isFollowing` per-viewer / typeahead / list 不缓存，KISS 同 relation「关系态 P0 不缓存」）；② TTL 10min（含 `weeklyNewCount` 时窗、无写触发失效 → 短 TTL 兜漂移 ≤15min，比 relation stats 24h 短）；③ **`SyncByBiz` 返回 affected tagIds 做精确失效**（新增+移除都即时反映 refCount，非 TTL 兜——初版「移除靠 TTL」被既有 `TestRefCountAcrossBiz` 打回，改精确失效）；④ Redis 故障读回源、写失效失败记日志不阻断（脏缓存至 TTL）。
**验证**: tag `go test ./...`（含 `TestDetail_CacheAside`，真 MySQL 3306 + Redis 6379）全绿；`make verify` 12 模块 workspace + GOWORK=off 双绿（go-redis v9.18.0 对齐、core redismocks 不破）；compose + tag config YAML `python yaml` 校验过；goimports 干净、`wire ./...` 重生成。
**待办**: search facet / 标签下文章、tag typeahead / list 缓存仍架构 P1。
**会话**: workflow:architect tag 缓存设计 + 落地
**发布**: 待上线（tag 随镜像；无 DDL/proto 变更；tag 容器需 `REDIS_PASS` + redis 可达）

## [2026-07-11] 标签「本周新增 X 篇」统计

**变更内容**: 承 `prd/tag/HANDOVER.md` §3.⑥，tag 详情页头部补「本周新增 X 篇」——近 7 天新增关联数（对齐原型 meta「128 篇内容 · 3.2k 人关注 · 本周新增 12 篇」）。
**影响范围**:
- **契约**：`Tag` 消息 +`weekly_new_count`（仅 `Detail` 计算，其余 RPC 恒 0；纯新增向后兼容，regen）。
- **tag 服务**：`TaggingDAO.CountRecentByTag(tagId, since)`（`COUNT WHERE tag_id=? AND created_at>=?`，跨 biz 与 ref_count 同口径，复用 `idx_tagging_tag_biz` 前缀，无新索引）；`repo.Detail(slug, since)` 解析 tag 后填 `WeeklyNewCount`；`service.Detail` 算 `since = now - 7d`（`weeklyNewWindow` 常量）；`grpc.toPb` 映射。e2e +1（近 7 天计 2 + 直插 8 天前旧关联验证滚动窗口排除）。
- **core BFF**：`domain.Tag` +`WeeklyNewCount` + `toDomainTag` + `tagDetailVO.weeklyNewCount`（Detail 已透传 Tag，无需改聚合逻辑）；handler 单测 +断言。
- **前端**：`types.TagDetail` +`weeklyNewCount`；`views/tag/detail` 头部 meta 追加「· 本周新增 Z 篇」（仅 Z>0 显示，避免非活跃标签的「0 篇」噪声）。
**技术决策**: ① **滚动 7 天窗口**（`now - 7*24h`）而非日历周——绕开周起点/时区边界，`created_at` 是绝对毫秒戳、tz 无关；② 计算而非存储——滚动窗口无法像 ref_count/follow_count 增量维护，读时 `COUNT`（per-tag 行数有界，MVP 免专用 created_at 索引）；③ 挂在 `Detail` 而非新 RPC——它是 tag+时间的属性（非 viewer 相关），未来短 TTL 缓存仍可用，与 isFollowing（per-viewer、独立 RPC）区别对待；④ 跨 biz 口径与 `ref_count`/「X 篇内容」一致（当前仅 article biz）。
**验证**: tag 模块 `go test ./...`（17 集成 e2e，真 MySQL，含窗口排除）全绿；core web 单测（tag handler +weeklyNewCount 断言）绿；`make verify` 12 模块 workspace + GOWORK=off 双绿、goimports 干净；前端 eslint+tsc+`next build` 绿。
**待办**: 关注数 / tag 详情缓存层（含本周新增）仍按架构 P1。至此 `prd/tag/HANDOVER.md` §3 前端 6 件套（①~⑥）全部完成。
**会话**: 接 prd/tag/HANDOVER §3.⑥ 本周新增统计
**发布**: 待上线（tag/core 随镜像 + 前端；proto 纯新增向后兼容；无 DDL）

## [2026-07-11] 标签关注订阅子系统（tag_follow）

**变更内容**: 承 `prd/tag/HANDOVER.md` §3.⑤，新增「用户关注标签」全链路——tag_follow 关注边 + 每标签关注数 + tag 详情页关注按钮/粉丝数。
**影响范围**:
- **契约**（`api/proto/tag/v1/tag.proto`）：`Tag` 加 `follow_count`；新增 `Follow`/`Unfollow`（`{uid,slug}→{changed,follower_count}`）/`FollowStatus`（`{uid,slug}→{is_following}`）3 RPC，regen。**向后兼容**（纯新增字段/方法）。
- **tag 服务**：`tag` 表加 `follow_count` 列（同 `ref_count` 的 per-tag 聚合，AutoMigrate 加 NOT NULL DEFAULT 0）；新表 `tag_follow`（uid+tag_id 关注边，**status 翻转不物理删**，镜像 `relation_follow`）+ `TagFollowDAO`（事务 `FOR UPDATE` 翻转 + `GREATEST(0,…)` 维护 follow_count + 回读返新计数 + `IsFollowing` 点查）；repo `Follow/Unfollow/IsFollowing`（slug→tag 解析，缺失→`ErrTagNotFound`，抽 `findBySlug` 复用）；service + grpc 3 RPC 实现。集成测 e2e +4（翻转/幂等/多用户累计/Detail 回显 follow_count/not-found）。
- **core BFF**：`domain.Tag` +`FollowCount`；`service.TagService.Detail(slug, viewerId)` 改签名，`errgroup` 并发聚合 Detail + FollowStatus（viewerId≤0 跳过、关注态失败非致命降级 false）+ 新增 `Follow`/`Unfollow` 委托；`web/tag.go` 加 `POST`/`DELETE /tag/:slug/follow`（需登录 `MustClaims`）+ `tagDetailVO` 加 `followCount`/`isFollowing`；`GET /tag/:slug` 由 `Public` 改 **`Optional`**（登录才算 isFollowing）；fake 桩 + mock 重生成 + web handler 单测 +4。
- **前端**：`api/tag.ts` `followTag`/`unfollowTag` + `types.TagDetail` +`followCount`/`isFollowing` + `FollowResult`；新组件 `components/tag/TagFollowButton`（2 态，镜像 relation `FollowButton` 视觉：teal 实心/白底描边 + 乐观切换 + 登录门控 + 失败回滚）；`views/tag/detail` 头部加关注按钮 + 「N 人关注」（服务端值为基准 + 本地乐观覆盖按 slug 隔离，避开 `set-state-in-effect`）。
- **视觉收口**（本轮 review 反馈）：① tag 详情页移除冗余面包屑「首页›标签›X」（PublicHeader 已提供返回路径；.pen 原型此前已删，本次重导出 `02-标签浏览页.png` 同步掉旧 PNG 里的面包屑）；② 统一标签 chip 配色到 `#F0FDFA`/`#0D9488`——`TaggedArticleCard`(默认态) + `TagInput` 原用 antd `token.colorPrimaryBg` 渲染偏灰，与 read/user 页及原型的浅 teal 不一致。
**技术决策**: ① 关注数存 `tag` 表新列而非另建 `tag_stats`——tag 本体就在本服务且已带 `ref_count` 聚合，relation 另建表只因它无实体表；② Follow/Unfollow 走 **slug**（公开标识，tag 服务内部解析 slug→id，省 core 一次往返）而非 tag_id；③ `Detail` 保持 viewer 无关的纯查询用于将来缓存（P1），isFollowing 走独立 `FollowStatus` RPC 不掺进 Detail；④ status 翻转 + `FOR UPDATE` + `GREATEST` 防负，全程镜像已验证的 relation follow 语义。
**待办**: 关注数 / tag 详情缓存层仍按架构 P1；⑥「本周新增 X 篇」（`tagging.created_at` 窗口计数）原型已画、后端未实现。
**验证**: tag 模块 `go test ./...`（domain + 16 集成 e2e，真 MySQL；含 follow 翻转/幂等/多用户/not-found）全绿；core `GRPCTagService` service_test +5（Detail 聚合/FollowStatus 降级/未登录跳过/tag 错误传播/Follow·Unfollow，新增 `web/grpcmocks/tag_mock.go` + `mk/mock.mk` 一行）+ TagHandler web_test +4，全绿——闭合 HANDOVER §六「core BFF service_test」遗留（tag 侧）；`make verify` 12 模块 workspace + GOWORK=off 双绿、goimports 干净、`wire ./...`/mockgen 重生成；前端 eslint + tsc + `next build` 全绿。
**会话**: 接 prd/tag/HANDOVER §3.⑤ 标签关注订阅
**发布**: 待上线（tag/core 随镜像 + 前端；proto 纯新增向后兼容；tag 服务 AutoMigrate 建 tag_follow + tag.follow_count 列，无手工 DDL）

## [2026-07-11] 鉴权路由自声明（@Public）+ 命名/结构标准化

**变更内容**: core HTTP 鉴权从「中央放行清单 + `/tag` 前缀巧合」演进为「路由就地声明访问级别」（`server.Public.GET`），顺带修前缀 footgun、统一命名/文件结构。
**影响范围**:
- `pkg/jwtx/middleware.go`: 中间件除具体 URL 外按 `ctx.FullPath()` 路由模板匹配（消除 `IgnoredPrefixes("/tag/")` 前缀 footgun）；新增 `WithResolver(func → ginx.Access)`，与中央 `IgnoredPaths`/`OptionalPaths` **并存**（三个 setter 未动，chat/migrator 零改动）。
- `pkg/ginx/router.go`（新）: `Access`（Protected/Public/Optional 枚举，零值=需登录 secure-by-default）+ `RouteRegistry.Lookup` + `Router`（内嵌 protected `scope` + `.Public`/`.Optional`）；`router_test.go` 5 例运行时验证放行/401/footgun。
- `internal/ioc/web.go`: 包级 `routeRegistry` 注入中间件 resolver + `InitWebServer` 建 `Router`；去 `IgnoredPrefixes`。
- `internal/web/tag.go`: `RegisterRoutes(*ginx.Router)`，`/tag/:slug`(+`/articles`) 走 `server.Public.GET`；`/tags/suggest`·`/tags/recommend` → `/tag/suggest`·`/tag/recommend`（不再靠单复数区分公开；gin v1.11 静态+带参同层已验）；`ExemptPrefix` 同步。
- `search/grpc/`: `search.go` 拆 `server.go`（`SearchServer` 骨架）+ `article.go`（article RPC + pb↔domain 映射），与 service/repository/dao 的 biz 命名对齐，为多 biz（user…）让路。
- `internal/cmd/backfill` → `backfill-search`（struct/injector/Makefile/wire 同步），确立 `cmd/<动词>-<宾语>/` 约定；删误提交的 `backfill.exe` + `.gitignore` 补 `*.exe`。
**技术决策**: ① @Public 与中央清单并存、非替换；② jwtx→ginx 依赖 acyclic；③ `SearchServer` 不改 `ArticleServer`（一个 proto service 单实现，user RPC 需挂进来）；④ 注释精简到功能级。
**待办**: core 集成测试的 test 中间件未挂 resolver（tag HTTP 集成测试将来需补，当前无）；`IgnoredPrefixes` doc 示例仍举已废弃的 `/tag`（方法按约定未动，示例待清）。
**验证**: `make verify` 12 模块 workspace + GOWORK=off 双绿；pkg/ginx router 运行时 5 例、internal web + backfill-search 单测、search 集成全过；`make -n backfill-search` 解析正确。
**会话**: workflow:ship 鉴权路由标准化批次
**发布**: 待上线（core/search 随镜像；无 proto/DDL 变更；路由改名前端未引用无破坏）

## [2026-07-11] tag 服务写路径批量化（消写侧 N+1）

**变更内容**: 发文/改标签的 tag 写链路去掉逐行往返——SyncTags 批量 Upsert、SyncByBiz 增删/ref_count 批量化，语义等价、gRPC 契约不变。
**影响范围**: 仅 `webook/tag/` 内部（gRPC 契约、mock、缓存策略均不动）——
- `repository/dao/tag.go`：`Upsert(单条)` → `UpsertTags(批量)`：一次 `CreateInBatches(OnConflict{DoNothing})` 建缺失 + 一次 `SELECT type=? AND slug IN` 回取全部真实 id，5~10 → 2 查询。
- `repository/dao/tagging.go`：`SyncByBiz` 事务内逐行 Create/Delete/ref_count UPDATE → `CreateInBatches` + `DELETE...IN` + ref_count 同向分组 UPDATE（`GREATEST(0,...)` 防负照旧），事务内 ~11 → ~4 语句、与标签数解耦，缩短热门 tag 行锁持有。
- `repository/tag.go`：`UpsertTags` + 新增 `toEntity`，按输入 slug 顺序重排回查结果（返回顺序=入参顺序）。
- `service/tag.go`：`SyncTags` 循环 Upsert → 组装 `[]domain.Tag` 一次批量解析。
- **注释优化**（同会话顺带，tag + search 两服务手写 Go，15 处）：删关联/废话注释（`同 relation`/`对齐架构 §X`/`与 core 同款`/`业界标准`/`镜像 core` 等），保留功能注释（幂等/降级契约/`GREATEST` 防负/候选窗口/索引语义）。纯注释、零行为变更，两模块 build+vet+test 复绿。
**技术决策**: ① 对齐 relation/interaction 既有 `clause.OnConflict{DoNothing}` + `GREATEST(0, cnt±?)` 惯例，非新造；② 批量 DoNothing + 回查比原逐个「SELECT→INSERT→冲突回查」并发更稳（`uni(slug,type)` 天然兜底）；③ 消费侧（core BFF errgroup 并发 + BatchBySlugs/FindByBizIds/BatchByBiz）已无 N+1，本次补架构此前只优化读侧、漏掉的写侧；④ 缓存仍按架构 P1，不在本轮。详见 `prd/tag/ARCHITECTURE.md` F4。
**验证**: tag 模块 `go build` + `go vet` + `go test ./...`（domain + 15 集成测试，含 ref_count 双向 / 超限不落库 / 重打标签幂等）+ `GOWORK=off` build/vet 五重绿；goimports 干净。集成测试连真库 `webook_test` 实跑通过。
**待办**: 无（行为等价优化；缓存层仍按架构 P1 待做）。
**会话**: workflow:perf tag 写路径批量化
**发布**: 待上线（tag 随镜像；无契约 / DDL 变更）

## [2026-07-11] tag/search 部署收尾：core BFF 去 ES 部署对齐 + 存量文章回填命令

**变更内容**: 承 `prd/tag/handover.md` 续做部署剩余项。① 把 P3 core BFF 重构（core 退为 tag/search 的 gRPC client、彻底去 ES/embedding）**同步到部署与配置**——此前只加了 tag/search 新服务块，漏改 core 自身的依赖图与死配置。② 新增存量 `published_article` → `webook-search` ES 索引的一次性幂等回填命令。
**影响范围**:
- **compose**（`deploy/docker-compose.yaml`）：`webook-core` 去掉 stale `depends_on: webook-es`（core 不再连 ES，却误等 ES 健康才启动）+ 去死 env `ES_PASS`/`QIANFAN_API_KEY`（core 已无消费者）；`webook-chat` 加 `depends_on: webook-search {service_healthy}`（chat 已直连 search gRPC）。
- **core 配置**（`internal/config/{local,dev,staging,prod,test}.yaml` 5 份）：删死块 `data.es` / `embedding` / `ollama`（`ioc/es.go` 已随 P3 移除、0 消费者）；`config/.env.example` 去 `QIANFAN_API_KEY`。`ES_PASS`/`QIANFAN_API_KEY` 仍由 search（及 migrator 的 `ES_PASS`）使用，故 `deploy/.env*` 保留不动。
- **backfill 命令**（`internal/cmd/backfill-search/`，wire 装配）：复用 core infra + 下游 gRPC client（DB 源库 + etcd 发现 + search/tag client），分页遍历 `published_article`、逐篇取当前标签（`tag.TagsByBiz`）+ 写 ES（`search.IndexArticle`）。幂等可重复跑。`make backfill-search`（`BACKFILL_ENV` 覆盖 yaml）。含 4 子测（批次终止/取标签降级/单篇失败计数/读源库致命错，不依赖真中间件、CI 可跑）。
**技术决策**: ① 依 core 既有先例——gRPC 下游（comment/interaction/relation）**不进 depends_on**、靠 etcd 惰性解析；故 core→tag/search 亦不加 depends（search 调用非致命降级，不该因 search 未就绪阻塞 core 启动）；chat→search 沿 chat→core 先例加 `service_healthy`。② backfill 走**发布写路径的下游契约、而非 migrator 管道**（后者裸列拷贝、无 embedding/enrichment/tags，会毁 kNN/facet）；语义是「按当前标签重建索引」，**绝不调 `SyncTags`**（空 names 会清掉已打标签）。③ `author_name` 两侧读路径均不带（与发布索引一致），search 服务端据 title/abstract 现算 `content_vec`。
**验证**: `make verify` 12 模块 workspace + GOWORK=off 双绿、goimports 干净、`make wire` 重生成 backfill `wire_gen.go`；backfill 4 单测过。7 份 YAML（compose ×2 + core 5 config）过 `python yaml` 校验。⚠ **未实跑**：docker CLI 未装（compose/镜像未 build/run）；backfill 需 MySQL+etcd+search/tag gRPC 在跑才能 e2e（本环境 ES/search 未起），当前仅编译 + 单测 + 照搬已验证的 publish 契约，**不算 e2e 通过**。
**待办**: ES:9200 + search/tag gRPC 起回后实跑 `make backfill-search` + `make -f mk/es.mk count` 复核（`search.Index` 对 ES 写失败静默降级、命令感知不到）；docker 环境 compose 起全套复核「core 无 ES 依赖」正常。前端（handover §3）+ core BFF service_test/web_test（handover §六）仍在 backlog。
**会话**: 接 prd/tag/handover 部署收尾（续作）
**发布**: 待上线（core 随镜像；配置/compose 变更部署时生效）

## [2026-07-10] ES 集群/认证支持 + xpack.security 强制密码 + ioc 重构

**变更内容**: core/migrator 的 ES 配置支持多节点（集群）+ Basic 认证；`deploy` 的 webook-es 开启 xpack.security 强制密码（Basic auth over HTTP，免 TLS）。顺带：修 v9 弃用 API、把建索引下沉业务层、mapping 抽成 JSON 文件。
**影响范围**:
- **客户端配置**：core `data.es` 由单 `addr` 改 `addrs`(列表，多节点即集群) + `username` + `password`；migrator `migrator.es` 补 username/password。core 5 份 + migrator 4 份 yaml 同步（local/test 明文 `elastic`/`elastic`，dev/staging/prod 走 `${ES_PASS}`）
- **服务端**：`docker-compose.yaml` webook-es `xpack.security.enabled=true` + `ELASTIC_PASSWORD=${ES_PASS}` + healthcheck 带 `-u`；core/migrator 容器转发 `ES_PASS`
- **部署变量**：`deploy/.env.*(.example)` 加 `ES_PASS`（命名随 `MYSQL_PASS`/`REDIS_PASS` 惯例；ES bootstrap 密码 ≥6 位）
- **运维**：`mk/es.mk` 全部 curl 加 `-u $(ES_USER):$(ES_PASS)`（默认 elastic/elastic，可覆盖）
- **弃用 API**：`NewTypedClient`/`NewClient(Config)` 已弃用 → 全线改函数式 `NewTyped`/`New` + `WithAddresses` + `WithBasicAuth`（core ioc、migrator ioc+test、sandbox demo 一致）
- **分层重构**：`ensureArticleIndex` 从 `ioc/es.go` 下沉 `dao/article_search.go`（建 article_v1 是业务关注点），ioc 只建 client；建索引失败经 `LoggerX` 告警不阻断启动；`NewElasticArticleDAO` 加 logger 参数，`wire ./...` 重生成
- **mapping 抽文件**：article_v1 与 sandbox demo 的 mapping 从内联 `map[string]any` 抽成 `article_index.json` / `doc_index.json`，`//go:embed` 读入
**技术决策**: ① ES「集群」= 多个 Addresses（非 redis 那种分片 client），addrs 列表即可，不引 `mode` 字段；② Basic auth over HTTP（`http.ssl.enabled=false`）单机内网免 TLS，与生产同构；③ 密码键名 `ES_PASS` 随 `*_PASS` 惯例；本地统一 `elastic`（`13520` 仅 5 位，ES 拒）；④ 建索引下沉 DAO 构造函数（wire 启动期跑一次）、mapping 用 go:embed 避免 JSON 硬编码。
**验证**: sandbox/es 连**已开安全**的本地 ES（elastic/elastic）**41/41 全绿**——与 core `es.go` 同款 `NewTyped+WithAddresses+WithBasicAuth`，佐证认证客户端可用；`make -f mk/es.mk status` 实测 article_v1（6 docs）；`make verify` 10 模块 workspace+GOWORK=off 全绿（含 wire 重生成）。
**待办**: **任何已部署环境（dev/staging/prod）** 开安全后，若 es-data volume 已存在，`ELASTIC_PASSWORD` 仅在空 volume 首次 bootstrap 生效——需 `docker volume rm webook-<env>_es-data` 重建或走 `_security/user/elastic/_password` 轮换，否则 core/migrator Basic auth 与 healthcheck `-u` 均失败、ES unhealthy 拖累依赖启动；且 ES 安全上线须与新 core/migrator 镜像同批（旧镜像不发凭据）。真机复验 core article 检索 + migrator ES 源/汇。
**会话**: 692a66ff-es-demo（接续）
**发布**: 待上线（core/migrator 随镜像；ES 镜像已在 compose 开安全）

## [2026-07-10] sandbox/es 全面 ES v9 集成 demo（TDD）

**变更内容**: 新增 `sandbox/es/`（独立 `module es`）——用官方 `go-elasticsearch/v9` TypedClient 的**全面用法示范**，交付形态为集成测试驱动的薄封装 `DocStore`（无 main）。**按能力分文件**（doc/store/index/document/bulk/search/aggregation/advanced），**41 个集成测试**连真实本地 ES 9.3.2 全绿，附完整 `README.md`。
**影响范围**:
- 新增 `sandbox/es/`：源文件 `doc.go`(实体+mapping) / `store.go`(DocStore+客户端工厂+结果类型+错误判定) / `index.go` / `document.go` / `bulk.go` / `search.go` / `aggregation.go` / `advanced.go`，测试一源一测 + `helper_test.go`（共享脚手架）+ `README.md`（独立模块，不入 webook go.work，零侵入）
- 查询/映射沿用项目 `internal/repository/dao/article_search.go` 的 `map[string]any` + `Raw(bytes.Reader)` 风格；响应走 TypedClient 强类型解析（`resp.Hits`/`resp.Aggregations`，`TypedKeys(true)` 解聚合 union）
- 覆盖场景：**A 索引管理**(建+mapping/exists/读mapping/删+幂等) · **B 文档CRUD**(index/create-409/get/get-404/部分update/update-404/delete/exists) · **C Bulk**(批量/混合/部分失败逐条409+404) · **D 搜索**(matchall/match/term/terms/bool/range/分页/排序/高亮/_source/空结果) · **E 聚合**(terms/stats/嵌套均值) · **F 计数**(全部/带query) · **高级**(PIT+search_after 深分页 / scroll 遍历 / mget / fuzzy·wildcard·prefix·multi_match / function_score·script_score / collapse)
- 清理上个会话残留的空目录 `sandbox/elasticsearch/`（`module es` 空壳，无源码）
**技术决策**: ① 客户端选 **v9** 而非项目原用 v8——demo 对齐**真实运行的本地 ES 9.3.2**（客户端 major 必须匹配 server major；webook 本次也已同步升 v9，见下条）；② 交付薄封装 + 集成测试而非 `main()`+打印，按能力分文件、源↔测一一对应，符合项目测试组织铁律；③ 样本文档实体命名 **`Doc`（中性通用文档）**，`Content` 用空格英文保证标准分词器下 match 确定，`Category/Tags` 用 keyword；④ `CreatedAt` 遵循项目时间铁律（Unix 毫秒 int64 + `date/epoch_millis`）；⑤ **每个测试独占索引**（名字由 `t.Name()` 生成），消除共享索引反复删建的偶发 `index_not_found` 竞态，并行安全；⑥ 引入 `testify`（项目统一断言库，test-only）。
**验证**: TDD 首轮 CreateIndex 打桩跑出真实断言失败（`Should be true / get mapping: 非预期状态码 404`）确认 RED 非编译错，实现后转绿；重构后全量 `go test ./...` **41/41 PASS**（连跑 3 次稳定，修掉初版共享索引偶发失败）、`go vet` 干净、gofmt 已跑。ES 环境 `127.0.0.1:9200`（无认证），可用 `ES_ADDR` 覆盖。
**待办**: 无（联动的 v8→v9 客户端 + docker 镜像升级见下条）。
**会话**: 692a66ff-es-demo（接续）
**发布**: 不涉及（sandbox demo）

## [2026-07-10] go-elasticsearch v8→v9 客户端迁移 + docker ES 镜像 9.3.2

**变更内容**: 本地/线上 ES 已到 9.x（本机 9.3.2），把 webook 的 ES 客户端从 `go-elasticsearch/v8 v8.19.6` 升到 `v9 v9.4.2`（客户端 major 必须匹配 server major），并把 `deploy/docker-compose.yaml` 的 `webook-es` 镜像从 `elasticsearch:8.12.2` 升到 `9.3.2`（dev/staging/prod 与本地对齐）。
**影响范围**:
- **`internal` 模块**：`ioc/es.go` + `repository/dao/article_search.go` import v8→v9（TypedClient 路径），`go.mod` 依赖 bump，`wire ./...` 重生成（ES client 是 Wire provider）
- **`migrator` 模块**：`ioc/engines.go`（低层 `NewClient`）+ `pipeline/source/es.go`（`Client`+`esapi`，search_after/aggs）+ `pipeline/sink/es.go`（bulk）+ `pipeline/source/es_test.go` import v8→v9，`go.mod` bump，`wire ./...` 重生成
- **`deploy/docker-compose.yaml:326`**：`elasticsearch:8.12.2` → `9.3.2`（single-node/xpack.security=false 设置 9.x 通用，无需改）
**技术决策**: ① v8→v9 是**纯 import 路径切换**——两种用法（internal 的 TypedClient、migrator 的低层 `Client`+`esapi`）在 v9 里 API 完全一致，逐文件核对签名（`Indices.Create/Exists`、`Search().Raw()`、`resp.Hits`、低层 `Search/Bulk`、`esapi.Response.IsError/StatusCode`）均在场；transport 仍 `elastic-transport-go/v8 v8.9.0` 不变；② 镜像选 9.3.2 对齐开发者本机实测版本，非盲追 latest。
**验证**: 两模块无 v8 残留（go 源 + go.mod + go.sum）；`cd webook && make verify` **10 模块 workspace + GOWORK=off 全绿**（含 wire 重生成）；migrator ES source httptest 测试全绿（FullScan/PKRange）；internal 的 TypedClient 用法由本次 `sandbox/es` demo（同 v9.4.2 客户端、同 `.Raw()`+TypedClient 模式）连真实 ES 9.3.2 **31/31 全绿**佐证运行时可用。
**待办**: 生产切 ES 9.3.2 后建议真机跑一遍 article 检索（BM25+kNN）与 migrator ES 源/汇对账，最终确认（本地无 docker，未起全栈集成）。
**会话**: 692a66ff-es-demo（接续）
**发布**: 待上线（下次 core/migrator 发版随镜像升级生效）

## [2026-07-09] redisx 统一 redis 配置 + worker/core 集群支持 + MGetPub 集群安全

**变更内容**: ① 新增 `pkg/redisx.Config`（完整 redis 连接 + 高级配置：mode/addr/addrs/password/池/超时/重试/集群路由，零值回落 go-redis 默认）+ `NewClient(cfg)`（按 mode 建单机/集群 UniversalClient、映射全部高级配置），收敛各服务内联 `Config{Addr,Password}`（原 7 模块 ~11 处各写一份）；② worker + core ioc 迁移到 redisx，支持 `mode: single|cluster`；③ **修 core MGetPub 集群 CROSSSLOT**——`MGet(多 key)` → Get 流水线（ClusterClient 按 slot 自动拆分，保分片不 CROSSSLOT）。
**影响范围**:
- 新增 `pkg/redisx/client.go`（`Config` + `NewClient`）+ `client_test.go`（单机/集群类型 + 高级配置映射断言，TDD RED→GREEN）
- **全部 11 处 redis 建点迁 `redisx.NewClient`**（worker/core/chat/comment/interaction/migrator/relation 的 ioc + 各 integration/setup），消除内联 `Config{Addr,Password}`；返回类型**按需收窄**（cache→`Cmdable`、锁→`UniversalClient`，遵循接口隔离）；`interaction`/`internal`/`internal-integration-setup` 的 wire 由 `*redis.Client` 双绑改 `Cmdable←UniversalClient` + `wire ./...` 重生成（`loadserver` 独立工具读 env 非 yaml、不在此列）
- `internal/repository/cache/article.go` MGetPub 改 Get 流水线；`article_test.go` + miniredis MGetPub 测试（原无测试，命中/miss/损坏/空全覆盖）
- worker + core 各 5 份 config yaml：`data.redis` 加 mode/addrs；**test→集群（7001-7003），其余单机**；worker 锁校准 `max_retries:-1`/`context_timeout_enabled` 移入 yaml，core 是共享 cache 用默认重试
**技术决策**: ① 连接 + 高级配置进 Config（yaml 好控制），worker 锁校准也移 yaml；② MGetPub 用 Get 流水线而非 hash-tag（后者把所有文章挤一 slot 成热点，流水线保分片）；③ 类型名保持 `Config`（idiomatic，如 `tls.Config`），文件名 `client.go`（对齐 grpcx `server.go`/`client.go`）；④ worker redis 锁专用（关重试）vs core 共享 cache（默认重试）刻意分道；⑤ 审计确认 core 唯一跨槽点是 MGetPub（`HMGet` 单 key、`Eval` 单 key、jwt/ratelimit 单 key 均安全）。
**验证**: 真 3 主集群探针实测——MGetPub 改前 `CROSSSLOT Keys in request don't hash to the same slot`、改后取回全部（Get 流水线）；`pkg/redisx` + article cache（含新 MGetPub）+ redislock cluster 测试全绿；`make verify` 10 模块（含 wire 重生成）全绿。
**待办**: core-cache 集群集成测（可仿 `RedislockClusterSuite`）；sharded SSUBSCRIBE 集群优化。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-09] redislock 集群真机验证 + 空 key 守卫

**变更内容**: 真 3 主 Redis 集群（7001-7003）实测 redislock——多 key Lua（release=lock+ch / fencing=lock+fence / fair=lock+queue+qts）靠 hash tag `{k}` 全部同槽、**无 CROSSSLOT**；**cluster 广播 pub/sub 唤醒生效**（阻塞 Lock 释放后近即时拿到、非等轮询）；100 goroutine 跨实例互斥只 1 赢。补 `internal/integration` 的 `RedislockClusterSuite`（skip-if-集群不可达）+ pkg 的 slot 一致性守卫单测。TDD 顺带发现并堵掉空 key 隐患：`TryLock/Lock("")` → 空 hash tag `{}` → 集群整键哈希 → 多 key CROSSSLOT，加 `ErrEmptyKey` 入口 fail-fast。
**影响范围**:
- `pkg/redislock/redislock.go`（+`ErrEmptyKey`）+`client.go`（TryLock/Lock 空 key 守卫）
- 新增 `pkg/redislock/consts_test.go`（slot 守卫：5 个 key-builder 同非空 hash tag）+ `client_test.go`（空 key 拒绝 RED→GREEN）
- `internal/integration/redislock_test.go`（+`RedislockClusterSuite`：多 key 无 CROSSSLOT / 跨 goroutine 互斥 / cluster pub/sub 唤醒；地址默认 7001-7003、`REDISLOCK_CLUSTER_ADDRS` 可覆盖、密码走 `REDISLOCK_REDIS_PASS`）
**技术决策**: ① 库对**正常 key** 本就 cluster-correct（`UniversalClient` + 全 key `{k}` hash tag），真机验证坐实、核心无需改；② 唯一隐患是空 key（空 tag 退化整键哈希），fail-fast 拒绝；③ slot 守卫纯字符串比对 hash tag（无需真集群、CI 可跑），防将来加第 6 种 key 忘 `{k}`；④ 集群集成测 skip-if-不可达（本地起 3 主 / CI 有集群才跑）；⑤ sharded SSUBSCRIBE 仍留后续（广播 pub/sub 真机已够用）；⑥ worker ioc cluster mode 切换仍不做（无集群部署，过早）。
**验证**: 真 3 主集群集成测 3 子测全 PASS（多 key/互斥/pub-sub）；`pkg/redislock` 37 单测全绿；`make verify` 10 模块全绿。空 key RED→GREEN：`TryLock("")` 原 ok=true/nil → 加守卫后 `ErrEmptyKey`。
**待办**: worker ioc cluster mode 接线（待真集群部署）；sharded SSUBSCRIBE；P5 多主 quorum。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] worker 锁专用 redis client 校准：关自动重试防 acquire 重复计数

**变更内容**: worker 的 redis client 仅用于 cron 分布式锁，`InitRedis` 补两项 go-redis Options——`MaxRetries=-1`（关自动重试）+ `ContextTimeoutEnabled=true`。默认 `MaxRetries=3` 在"命令已执行但响应丢失"时会重发整条命令，而 acquire 脚本 `hincrby` **非幂等** → 重复 +1 → 计数虚高、Unlock 减不到 0、锁滞留到 lease 过期（幻觉持有，别副本这段抢不到、可能跳 cron tick）。`refresh`(pexpire)/`release`(hexists 守卫) 幂等无此患，唯 acquire 有 → 整体关重试最稳。
**影响范围**: `worker/ioc/redis.go`（仅此一处；与 chat/core 共享 redis client **刻意分道**——它们要 cache 的透明重试，锁不能要）
**技术决策**: ① 锁的瞬时错误交调用方降级 + watchdog 自身重试循环，不靠 go-redis 静默重试；② **不收紧 ReadTimeout**——对 cron 快失败会让 tick 直接跳过（不如晚点跑），默认 3s 已有界；③ cron 低并发（每 30s 一次）+ `ConnMaxIdleTime` 30min，默认池足够、连接常温，不调池；④ perf 定位：先实测否决了 token RandPool（crypto/rand 57ns 已快、池反而 -3×），Redis 往返是延迟下限改不掉，客户端唯一实招是这个 acquire 幂等性配置。
**验证**: worker build + `make verify`（10 模块 workspace/GOWORK=off）全绿。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] redislock 压测补全：Go 并发 harness + pub/sub-vs-poll 基准 + JMeter 公平锁

**变更内容**: 补齐 §7 压测——① 新增 `loadtest/loadtest.go` 进程内并发 harness（`Config.Run`→`Report`，多 goroutine 真 Redis 抢锁，校验 MutexViolations/FenceMonotonicBreaks 必须 0 + QPS/延迟分位），入口 `TestLoad`（真 Redis 不可达则 skip）；② 补 §7.1 缺的 `BenchmarkBlockingLock_PubSubVsPoll`（量化 pub/sub 唤醒 vs 纯轮询 hand-off 延迟）；③ loadserver + JMeter 计划接入公平锁（`fair=true`）。
**影响范围**:
- 新增 `pkg/redislock/loadtest/loadtest.go`+`loadtest_test.go`（§7.2 Go harness）
- `pkg/redislock/bench_test.go`：+`BenchmarkBlockingLock_PubSubVsPoll`（poll 臂手动轮询 vs pubsub 臂 Lock）
- `loadtest/cmd/loadserver/main.go`：`acquireOpts` +`fair=true`、新增 `POST /reset`（清零计数 + 释放残留句柄，多轮压测免重启；README 曾引用但一直缺实现）；`main_test.go` 抽 `acquireQ` 助手消重 + `TestLoadServer_FairParam`/`TestLoadServer_Reset`
- `loadtest/jmeter/redislock.jmx`（+`fair` 属性）+`README.md`（fair 压测示例/属性/端点；软化 `activeHolds` 收尾残留措辞 + 补 `/reset` 端点行）
- **修 `wait_seconds` 死指标**（`prometheus/builder.go`）：查压测 `/metrics` 发现 `wait_seconds` 恒 0——原仅 `Lock()` 观测，而全部消费者（cron/worker/loadserver）走 `TryLock`，且 `TryLock+WaitTime`/公平排队阻塞时间也不可见。改为 `TryLock` 也观测获取耗时（含阻塞排队），Help 拓宽；`builder_test.go` 加 `TestBuild_TryLockWait_Histogram` + Lock 测改增量断言
**技术决策**: ① Go harness 与 JMeter HTTP 壳互补（前者 go test 直跑、后者外部工具驱动，同一不变量口径）；② pub/sub-vs-poll 基准用"poll 臂手动 TryLock 轮询 + pubsub 臂 Lock(大 retryInterval)"隔离两种唤醒机制；③ 公平锁经 loadserver 仅验互斥（HTTP 无状态难判 FIFO 入队序，FIFO 由单测/集成/harness 保证）；④ 重入不接 loadserver（token=ownerId 固定→句柄 map 撞车 + 同 owner 多持有会误报 mutexViolation，语义不合 HTTP 壳）。
**验证**（真 Redis 127.0.0.1:6379，§7.3 判据全达标）:
- `TestLoad` 三模式 MutexViolations=0 / FenceMonotonicBreaks=0：default(64g/4key QPS≈1453、p99 2.5ms)、fair(32g/2key **busy=0** 全排队、p99 221ms)、fencing(64g/4key 令牌单调 0 破)
- `BenchmarkBlockingLock_PubSubVsPoll`：poll 51.7ms/op vs pubsub 5.9ms/op（**~8.7× hand-off 提速**，坐实 §3.4 收益）
- micro-bench（真 Redis）：uncontended 186µs、watchdog +9µs、fencing +1µs、reentrant depth 线性
- loadserver 自测（真 Redis）fair busy=0/mutex=0；`make verify` 全绿
**待办**: `BenchmarkQuorum3`（§7.1，绑 P5 多主）；fair FIFO 的 JMeter 侧校验（当前靠库测保证）。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] redislock P4-10 公平锁：WithFair FIFO 排队 + 死等待者逐出

**变更内容**: 按 §3.5 落地 P4 任务 10（本库最复杂、正确性风险最高能力）——`WithFair()` 公平锁：被占时按开始等待先后 FIFO 排队，杜绝抢占式下早等者被后来者反复插队饿死。数据用 `queue`(list FIFO 队列) + `qts`(zset ownerToken→逐出 deadline)；`fair_acquire.lua` 原子四步（清理队头死等待者 → 重入 → 锁空闲且队头是我则出队获取 → 否则入队尾+刷新 deadline）；释放复用 `release.lua` 的 publish 唤醒，唤醒后只有队头能在脚本成功，从而 FIFO。优雅放弃（ctx 取消 / WaitTime 到点）主动 `fair_cancel.lua` 出队，崩溃者靠 deadline 逐出兜底。
**影响范围**:
- 新增 `scripts/fair_acquire.lua`+`fair_cancel.lua`；`script.go`(+2 embed/script)；`consts.go`(+queueKey/qtsKey)
- `options.go`：`WithFair()`+`fair` 字段+`heartbeatMs()`(3×retryInterval)+`validate()`
- 新增 `fair.go`：`acquireFair` / `dequeueFair` / `ErrFairFencingUnsupported`
- `client.go`：`tryAcquire` 改 switch（fair / fencing / 普通）+ `TryLock`/`Lock` 入口 `validate()` + 非阻塞 fair 未拿到即 `dequeueFair`
- `pubsub.go`：`blockingAcquire` 放弃路径（WaitTime 到点 / ctx 取消）主动 `dequeueFair`
- 单测 `fair_test.go`（入队 / FIFO 顺序 / 死等待者逐出 / 公平重入 / fair+fencing 报错 5 例）；集成测 `redislock_test.go`（真 Redis 4 等待者 FIFO）
**技术决策**: ① deadline=3×retryInterval **每次尝试刷新**（心跳）——活等待者永不误逐、崩溃者约 3×retryInterval 被逐，统一适配 Lock（无 waitTime）与 TryLock+WaitTime；② 优雅放弃主动出队（不等 deadline，减后面人等待）+ 崩溃靠 deadline 兜底，双保险；③ fair+fencing **fail-loud 报错**（不静默丢 fencing 安全）；④ now 用 Go 传（miniredis 兼容，逐出 best-effort 容忍客户端时钟微偏）；⑤ queue/qts 自清理（Redis 空 list/zset 自动删键，无泄漏，release.lua 无需改）；⑥ 复用 P4-9 的 `blockingAcquire` 订阅唤醒循环，fair 只换获取 Lua。
**验证**: `pkg/redislock` 35 单测全绿（新增 5）；FIFO 顺序测试 4 并发等待者严格 [0,1,2,3]；注入 deadline 过期的死队头后被逐出、活获取者拿到；`make verify`（fmt + 10 模块 workspace/GOWORK=off）全绿；集成测编译校验（真 Redis 手动 `docker compose up redis` 跑）。RED→GREEN：先 `WithFair` 存根跑出确定性 RED（阻塞等待者不入队、`queue` 恒空 → `Eventually` 2s 超时），再实现 fair_acquire 转绿。
**待办**: fair 微基准（§7.1 未列，loadtest harness 的 `Fair` 字段可后补接入）；sharded SSUBSCRIBE 集群优化；fair+fencing 组合（需专门 fair+fence 原子脚本）；P5 多主 quorum + 部署。至此 **P1–P4 全部落地**。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] redislock P4-9 阻塞增强：Lock/WaitTime 走 pub/sub 唤醒（去轮询死等）

**变更内容**: 按 `prd/redislock/ARCHITECTURE.md` 落地 P4 任务 9——`Lock` 与 `TryLock+WaitTime` 的阻塞获取从"固定间隔轮询"升级为 **pub/sub 唤醒**：先 `SUBSCRIBE redislock:{k}:ch`（订阅先于试获取，堵住"释放信号在订阅前发出"的丢失窗口）→ 试获取 → 被占则 `select{释放消息唤醒 / min(pttl,retryInterval) 兜底轮询 / ctx.Done}` 后重试；所有退出路径 defer 退订防连接/goroutine 泄漏。释放后近即时拿到（原轮询要等满 retryInterval），持有者崩溃不 publish 时靠 pttl 兜底在锁自然过期附近重试。
**影响范围**:
- 新增 `pkg/redislock/pubsub.go`：`blockingAcquire`（订阅生命周期 + 唤醒/兜底/取消三路 select）+ `blockWait`（min(pttl,retryInterval) 且不超 deadline）
- `pkg/redislock/client.go`：`acquire`→`tryAcquire`（被占时 surface 剩余 pttl 供兜底计算）；`TryLock` 无 WaitTime 走单次 tryAcquire、有 WaitTime 走 blockingAcquire；`Lock` 走 blockingAcquire（deadline 零值=仅 ctx 上限）
- `pkg/redislock/fence.go`：`acquireFencing` 同步改为 surface pttl 的签名
- 单测 `client_test.go`：pub/sub 唤醒近即时（大 retryInterval 下释放后 <1s）+ 无订阅 goroutine 泄漏（30 轮 goroutine 数持平）
- 集成测 `internal/integration/redislock_test.go`：`TestBlockingLock_HandsOff` 改用 retryInterval=5s，成为真 Redis 的 pub/sub 收益证明
**技术决策**: ① 订阅先于试获取（§3.4），消除 lost-wakeup 窗口；② 兜底 `min(pttl,retryInterval)`：pub/sub 是快路径，pttl 兜底覆盖"持有者崩溃不 publish→锁 TTL 自然过期"的慢路径，兼顾延迟与正确；③ `tryAcquire` 统一 surface pttl（acquire.lua/fence.lua 本就返回剩余 pttl，之前丢弃），阻塞与非阻塞共用一处获取逻辑；④ 广播 `SUBSCRIBE`（单机/集群均正确），sharded `SSUBSCRIBE`（集群省广播）留待集群消费者时优化，已在 pubsub.go 注明；⑤ 非阻塞 TryLock（无 WaitTime）不订阅、零额外开销。
**验证**: `pkg/redislock` 30 单测全绿（新增 2：pub/sub 唤醒 0.11s vs 原轮询 5.01s、无泄漏 30 轮）；miniredis 探针证实其投递 Lua PUBLISH 给订阅者（故唤醒可单测）；`make verify`（fmt + 10 模块 workspace/GOWORK=off）全绿；集成测编译校验（真 Redis 手动跑）。`-race` 需 cgo 本地缺失→CI 跑（§6）。RED→GREEN：先写唤醒测试对轮询代码跑出 `5.001s not less than 1s`，再实现 pub/sub 转 0.11s。
**待办**: P4 任务 10 公平锁 `WithFair()`（§3.5，大、正确性风险最高，依赖本次 pub/sub 唤醒机制）下一轮单独 TDD；sharded SSUBSCRIBE 集群优化待集群消费者；P5 多主 quorum 未动。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] redislock P3 可重入：WithReentrant 显式 ownerId 跨 goroutine 共享持有者身份

**变更内容**: 按 `prd/redislock/ARCHITECTURE.md` 落地 P3——新增 `WithReentrant(ownerId)` 选项：同一 ownerId 的多次获取即重入（hash 计数 +1），释放需与获取次数相等才真正释放；Go 无稳定 goroutine id（ADR-2），故重入身份显式传，协作 goroutine 传同一 ownerId 即共享同一临界区。默认（不传）每次获取用随机 token、天然不可重入（opt-in，零回归）。存储/Lua 零改动（acquire/release/fence.lua 的 hincrby 重入计数在 P1+P2 已就绪），仅补获取时的 token 解析。
**影响范围**:
- `pkg/redislock/options.go`：新增 `WithReentrant(ownerId)` + `lockConfig.ownerId` 字段
- `pkg/redislock/client.go`：新增 `resolveToken(cfg)`——ownerId 非空用它当 token，否则随机 `newToken()`；`TryLock`/`Lock` 改用之
- 单测 `client_test.go`（同 owner 重入 / 释放 N 次 / 跨 owner 互斥 / Token==ownerId / 默认 opt-in / 跨 goroutine 共享 6 例）+ `fence_test.go`（重入不 bump fencing 计数器 1 例）
- `bench_test.go`：新增 `BenchmarkReentrant`（非重入基线 vs depth 2/4/8 重入开销，§7.1）
- `internal/integration/redislock_test.go`：真 Redis 重入 + 跨 owner 互斥用例
**技术决策**: ① 重入身份显式 ownerId（ADR-2，Go 无 goroutine id 不 hack），默认随机 token 保持 opt-in、零回归；② token 解析集中 `resolveToken` 一处，Lua/存储不动（重入计数天然在 hash `hincrby`）；③ 命名遵循本仓铁律 `Id` 风格（`ownerId` 非 `ownerID`）；④ 重入 × fencing：重入不 bump 单调计数器、重入句柄 `Fence()`=0（沿用首次令牌，fence.lua 既有行为，补测试固化）。
**验证**: `pkg/redislock` 28 单测全绿（新增 7 例：6 重入 + 1 fencing 交互）；`make verify`（fmt + 10 模块 workspace/GOWORK=off）全绿；`BenchmarkReentrant` 冒烟跑通；集成测编译校验（`go vet` 通过，真 Redis 手动 `docker compose up redis` 跑）。RED→GREEN 严格执行：先 stub 选项让编译、断言失败（second ok=false / Token 为随机 UUID）证明行为缺失，再接 `client.go` 转绿。
**待办**: `reentrant_depth` 指标（贯穿 §5 task 14）暂缓——正确实现需每次获取多一次 HoldCount 往返（污染热路径）或改 acquire.lua 返回协议（风险波及 P1 全部测试），性价比低，待有消费者需求再评估；P4 pub/sub 阻塞+公平锁、P5 多主 quorum 未动。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] redislock P1+P2 实现：改名去 bsm + 自研 Lua 核心 + 单机/集群 + fencing

**变更内容**: 按 `prd/redislock/ARCHITECTURE.md` 落地 P1+P2——`pkg/redislockx` 重命名重写为纯自研 `pkg/redislock`（移除 `bsm/redislock`）；自研 Lua 核心（获取/释放/续约，`hash{ownerToken:重入计数}` 存储 + hash-tag key）；watchdog 搬入 P0 修复（网络错误距上次成功≥租约视同丢锁→OnLost+退出）；参数经 Options（waitTime/leaseTime/watchdogTimeout/retryInterval/fencing）+ 查询方法 + ForceUnlock；单机/集群同一 `NewClient(redis.UniversalClient)`；P2 fencing（`WithFencing`+`Fence()` 跨获取单调 + 资源侧 `FenceAccepted` helper）。cron 行为零回归。
**影响范围**:
- 新增 `pkg/redislock/`：`redislock.go`(接口 Client/RedisLock)+`consts/options/script/factory/client/lock/fence.go`+`scripts/{acquire,release,refresh,fence}.lua`+`prometheus/`+`mocks/`+单测+`bench_test.go`(§7.1 微基准)+`loadtest/cmd/loadserver`(§7.2 HTTP 壳 /acquire·/release·/stats·/metrics)+`loadtest/jmeter/`(JMeter 计划)
- 删除 `pkg/redislockx/`（含 mocks/prometheus/tests）
- 迁移消费者：`pkg/cronx/wrapper.go`(+test)（`TryLock(ctx,key,ttl)`→`WithWatchdogTimeout`）、`worker/ioc/{lock,cron,redis}.go`（`InitRedis`/`InitLockClient` 入参 `redis.Cmdable`→`redis.UniversalClient`）、worker `wire_gen.go` 重生成、`internal/service/ranking.go`(注释)、`internal/integration/{redislock,cronx}_test.go`（重写 + 新 key 模型/句柄查询）、`internal/integration/setup/redis.go`(→UniversalClient)、`mk/mock.mk`、`pkg/go.mod`(去 bsm)
- 指标扩展 `webook_lock_fence_issued_total`
- 文档：`worker/CLAUDE.md`、`prd/config/config-architecture.md` 引用改名
**技术决策**: ① 自研 Lua 全原子（4 脚本），hash 模型天然支持重入计数；② fencing 计数器持久不过期（过期→单调断裂）、仅全新获取 INCR；③ watchdog `innerMu` 串行化脚本执行 + `stop`/`Once` + P0 三分支；④ `NewClient(UniversalClient)` 单机/集群透明、hash-tag `{k}` 化解 CROSSSLOT；⑤ `Refresh` 去掉 ttl 参数（用句柄租约），签名 `(ctx,key,opts...)` 干净；⑥ 句柄接口领域名 `RedisLock`（非裸 `Lock`）。
**验证**: `pkg/redislock`+`prometheus`+`cronx` 单测全绿（miniredis，含 watchdog wall-clock + fencing 单调）；`make verify`（fmt + 10 模块 workspace/GOWORK=off）全绿；benchmark 量化开销（uncontended≈200µs、watchdog +19µs、fencing +6.5µs 每 op）；真 Redis 压测经 JMeter HTTP 壳 loadserver（`/acquire`+`/release`+`/stats`+`/metrics`）——单 key 极限竞争下 mutexViolations/fenceMonotonicBreaks/watchdog_lost 全 0，httptest 并发自测 + 真二进制 smoke 佐证。集群集成测（真 ClusterClient）随 CI。
**待办**: P3 可重入（`WithReentrant` 跨 goroutine）、P4 pub/sub 阻塞+公平锁、P5 多主 quorum+部署——按消费者需求逐段推；集群 ClusterClient 集成测（需真 ClusterClient）随后补。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-08] redislock（原 redislockx）安全可靠锁库重设计方案（设计）

**变更内容**: 出「`pkg/redislockx` → 重命名并重设计为纯自研 `pkg/redislock`」方案：补齐可重入 / 公平锁 / pub-sub 阻塞 / 多主 quorum / fencing 五能力，支持单机 + 集群 + 多主三拓扑；获取沿用本仓 `Client.TryLock/Lock → 句柄` 模型、句柄接口领域名 `RedisLock`、参数（waitTime/leaseTime）经 Options；配 benchmark + 真实压测 harness（互斥/公平/单调不变量校验）验证有效性。仅设计文档，未动代码。
**影响范围**: 新增 `prd/redislock/ARCHITECTURE.md`（三拓扑 / 五能力 Lua 草图 / fencing 资源侧契约 / 风险 / 5 阶段任务 / 前瞻·分层·wire 三章）。当前唯一消费者 worker cron（经 `pkg/cronx` → `TryLock`）。
**技术决策**: ① 自研 Lua 核心、移除 bsm/redislock（可重入 hash / pub-sub / 公平 / fencing 均非 bsm 能力），移除后 `redislock` 包名腾出故改名；② fencing 是唯一真安全（单调令牌 + 资源侧校验），多主/集群只解决可用性；③ 重入身份显式传 ownerID（Go 无稳定 goroutine id）；④ 存储统一 hash 模型；⑤ 沿用本仓 `Client.TryLock/Lock` 获取模型，句柄接口用领域名 `RedisLock`（非裸 `Lock`，避免与方法 `Lock()` 同词），纯自研、不引入外部术语；⑥ 单机/集群同一 `NewClient(redis.UniversalClient)`，key hash-tag 化解集群 CROSSSLOT；⑦ watchdog 沿用 ttl/3（含已修 P0：网络错误超 ttl 视同丢锁）。
**待办**: 实施 P1（改名+自研核心+单机/集群）→ P2（fencing）→ P3（重入）→ P4（pub-sub+公平）→ P5（多主+部署重构）；建议先 P1+P2 上线。**落地时同步**（描述活状态、不提前写）：`webook/worker/CLAUDE.md`、`prd/config/config-architecture.md` 的 `redislockx` 引用改 `redislock`。另：本会话 P0 watchdog 修复 + §8 命名对齐（`RedisLock`/`safe*`→`fire*`）已完成待提交（独立 commit）。
**会话**: 260708-redislock-重设计
**发布**: 待上线

## [2026-07-07] Go 多模块化 + go.work 落地实现

**变更内容**: 按 `prd/go-workspace/ARCHITECTURE.md` 落地——`webook/` 单模块拆为 10 个独立 module（每服务 + pkg/api/shared）+ `go.work`；历史占位模块名 `github.com/webook` → `github.com/boyxs/train-go/webook`（对齐真实仓库）。一个提交完成。
**影响范围**: 369 `.go` 前缀重命名 + 6 `.proto` go_package；10 模块 `go.mod`（服务 `require`+`replace` 本地兄弟：migrator 无 api、worker 含 internal）+ `go.work`/`go.work.sum`；root `go.mod`/`go.sum` 删除；7 Dockerfile（`GOWORK=off`+replace 分层 COPY）；7 CI（`cache-dependency-path=webook/**/go.sum`、PR paths 补 `shared`）；根 `Makefile`/`mk`（MODULE 硬编码、wire/mockgen 逐模块 cd）；`webook/CLAUDE.md`、`.claude/rules/coding-rules.md`、`prd/migrator/02-architecture.md`（布局说明加注）。
**技术决策**: ① 依赖版本零漂移——各模块 go.mod 从原版本 seed 再 tidy，避免 fresh tidy 抓 latest 误升级 grpc/gin/sarama/go-redis；② internal 保留原名不改 core（零 import 改写）；③ `go.work` + `replace` 双保险（`go mod tidy`/Docker/CI 全离线确定性）；④ 前缀迁移作为独立首步在单模块态一次机械替换；⑤ 修复 `pkg/errs` 全仓 sentinel guard 的 `repoRoot`：锚点 `go.mod`→`go.work`（多模块下才扫得全仓 60 个 reason）。
**验证**: 10 模块 build+vet（workspace / GOWORK=off / -mod=readonly 三态）全绿；7 Dockerfile 构建命令编译通过（含 internal k8s tag、worker+internal）；chat Dockerfile COPY 集 scratch 模拟产出 Linux 二进制；pkg 单测全绿（含全仓 guard）+ internal service/web 单测全绿；离线 tidy 通过。
**待办**: 集成测试 + 真 `docker build` + CI 实跑（本地无 MySQL/Redis/Kafka/etcd/docker）——push 后由 CI 验证；`.gitignore` 的 `go.work` 忽略行可删（go.work 已 tracked 故失效）。
**会话**: 260707-go-workspace-多模块化
**发布**: 待上线

## [2026-07-07] Go 多模块化 + go.work 架构方案（设计）

**变更内容**: 出「`webook/` 单模块 → 每服务 + `pkg`/`api`/`shared` 各自独立 `go.mod` + `go.work`」架构方案；并决定顺带把历史占位模块名 `github.com/webook` 对齐真实仓库 `github.com/boyxs/train-go/webook`。仅设计文档，未动代码。
**影响范围**: 新增 `prd/go-workspace/ARCHITECTURE.md`（10 模块拓扑 / `go.work`+`replace` 契约 / Docker·CI·wire·mock 配套改造 / 6 阶段迁移 / 风险 / 决策）。侦察确认依赖图无环、服务间零耦合（唯一跨服务边是 worker 契约测试 → `internal/events/interaction`）、根模块可解散（根目录无 `.go`）。
**技术决策**: ① 模块前缀对齐真实仓库 `github.com/boyxs/train-go/webook`，作为拆分前**独立第一步**在单模块状态一次机械替换（369 `.go` + 6 `.proto` 的 `go_package` + CI `MODULE` env），`go build` 兜底；② `internal/` 保持原名（`.../webook/internal`），零 import 改写，不改 `core`（Go internal 可见性基于路径前缀，兄弟模块仍可 import）；③ `go.work` + 各 `go.mod` 加 `replace`（tidy/Docker/CI 离线确定性解析），Docker 用 `GOWORK=off`；④ 根 `go.mod`/`go.sum` 解散；⑤ `go.work`/`go.work.sum` 提交进 git（monorepo 实践）。
**待办**: 实施——Phase 0 前缀迁移（单独提交）→ P1 叶子模块（api/shared/pkg）→ P2 建 `go.work` → P3 逐服务模块（init+replace+tidy+use，worker 特例加 internal）→ P4 删根模块 + `go work sync` → P5 改 Docker/CI/Makefile/mk + 同步 `webook/CLAUDE.md`·`coding-rules.md` → P6 端到端验证。CLAUDE.md/coding-rules 的多模块规则**待 Phase 5 落地时同步**（描述活状态，不提前写）。
**会话**: 260707-go-workspace-多模块化
**发布**: 待上线

## [2026-07-06] 用户关系（relation）原型↔实现对齐 + 数据补齐（消 N+1）

**变更内容**: 复盘 relation 原型与实现不一致并双向对齐——改原型（他人主页头部→PublicHeader、去 `@handle`/动态 Tab/文章缩略图；关注粉丝页补个人信息卡）+ 补后端数据匹配原型（列表每人 关注/粉丝数、粉丝「关注了你·时间」、文章卡评论数）；新增 comment `BatchCountComment` 一次 GROUP BY 消除评论数聚合的 N+1。
**影响范围**:
- 原型: `prd/relation/relation.pen`（他人主页/关注粉丝两 frame）+ 重导 `01/02` PNG + `PRD.md`（原型资产 + 对齐决策）
- 后端(core): `internal/web/relation.go`（followees/followers 加 `BatchGetStats` 每人计数 + `followerItemVO.createdAt`）；`internal/web/article.go`（`/article/reader/author` 加 `commentCnt`，一次 `BatchCountComment`）
- 后端(comment): `api/proto/comment/v1`（+`BatchCountComment`）+ gen；comment dao/repo/service/grpc 加 `BatchCount`（GROUP BY biz_id）；mocks 重生成；comment dao 真库测试
- 前端: types（`FolloweeItem`/`FollowerItem` 加计数+`createdAt`；`ReaderArticle` 加 `commentCnt`）；`FollowButton`（`followLabel`「回关」）；`FollowList`（每人计数副行 + 粉丝「关注了你·时间」+ 已加载 X/总数）；`profile` 传 total；`detail` 文章卡「点赞·评论」
**技术决策**: ① 评论数用批量 `BatchCountComment`（一次 GROUP BY）而非 N 次 `CountComment`，消 N+1 跨服务；② 每人计数复用 relation 现成 `BatchGetStats` 一次批量；③ 他人主页保持公开（PublicHeader）、关注粉丝保持组合页（含个人信息卡）；④ 无数据源项（`@handle`/动态/缩略图）改原型删除；⑤ **分层规范化**：他人主页文章的互动/评论/获赞聚合从 web 下沉到 `ArticleReaderService.AuthorArticles`（service 持 comment gRPC client、返回 `domain.ArticleWithStats`），web `Author` 仅调用 + `slicex.Map` 映射 VO；`webook/CLAUDE.md` 补「三层职责边界」规则（web 搬运 / service 编排聚合 / repo 存取）；⑥ 遗留数据 `user.created_at` 为 NULL 导致「加入时间」不显示，按 §5 约定回填；⑦ 按新规则清历史债：comment / relation 网关 handler 的聚合下沉到新 `service.GRPCCommentService` / `GRPCRelationService`（持下游 gRPC client + userSvc + producer，聚合评论树/关系列表 + 昵称 + 计数 + 事件），web handler 瘦身为「调 service + `slicex.Map` 映射 VO」，评论聚合测试迁到 service 层；`abstractFromContent` 提取为 `pkg/stringx.Abbreviate` + `domain.Article.DisplayAbstract()`；CLAUDE.md 规则明确「接入层 = web/ 或 grpc/」（gRPC server 等同 web 层）。
**待办**: 移动端未运行态目检；获赞后续 denormalize；私信仍为 P1 占位。
**会话**: 260706-relation-用户关系系统
**发布**: 待上线

## [2026-07-06] 用户关系（relation）前端全量 + 他人主页后端补齐

**变更内容**: relation 前端落地（关注按钮 4 态 / 关注·粉丝 Tab / 黑名单页 + 拉黑弹窗 / 他人主页）；为「他人主页」补 core 只读端点（公开用户信息 + 按作者已发布文章 + 获赞聚合）。
**影响范围**:
- 后端(core)：`internal/repository/dao/article_reader.go`(+PageByAuthor/CountByAuthor/ListIdsByAuthor)、`internal/repository/article_reader.go`、`internal/service/article.go`(reader 层同名方法)、`internal/web/article.go`(POST `/article/reader/author` + ReaderArticleVO.likeCnt + likedTotal)、`internal/web/user.go`(公开 POST `/user/info`)、`internal/ioc/web.go`(两端点入 IgnoredPaths)、mocks 重生成。
- 前端(webook-fe)：`types/relation.ts`+`api/relation.ts`(8 端点)、`types/user.ts`/`api/user.ts`(UserInfo)、`types/article.ts`/`api/article.ts`(ReaderArticle/AuthorArticlesResult)、`components/relation/{FollowButton,UserCard,FollowList}.tsx`、`hooks/useCursorList.ts`、`utils/format.ts`、`views/user/{profile,blocklist,detail}.tsx`、`app/(main)/user/profile`(Suspense)、`app/(main)/user/settings/blocklist`、`app/(auth)/user/[id]`。入口打通：`components/layout/Header.tsx`(用户菜单加「黑名单」)、`components/comment/CommentItem.tsx`(评论人头像/昵称点进他人主页)、`views/article/read.tsx`+`types/article.ts`(文章详情页新增作者行：头像/昵称点进他人主页 + 就地关注按钮，作者不再依赖评论才可达)。悬浮卡：`components/relation/UserHoverCard.tsx`(hover 头像/昵称出基本信息卡 + 关注/发私信便捷操作,懒加载+按 uid 模块级缓存+mouseEnterDelay 防划过触发),挂到评论区作者(`CommentItem`)与文章详情作者(`read.tsx`)。
**技术决策**: ① 他人主页公开可读（置 `(auth)` 组 + PublicHeader，写操作走 FollowButton 的登录校验），对齐 article 读页；② 关注/粉丝/黑名单游标分页抽 `useCursorList`；③ 关注按钮 4 态精确对齐 relation.pen（padding[8,20]/lucide/token 色），乐观更新 + 失败回滚；④ 获赞无 per-user 计数器，实时聚合作者已发布文章 likeCnt；⑤ 昵称/简介经公开 `/user/info`，relation 列表昵称仍由 core 聚合。
**待办**: 获赞后续 denormalize 到计数器；私信为占位提示（用户间 DM 属 P1 未建，**不接** AI `/chat`——二者无关）；文章卡展示点赞+浏览（评论数属 comment 服务，未二次聚合）；移动端用响应式类适配，未在运行态浏览器逐页目检。
**会话**: 260706-relation-用户关系系统
**发布**: 待上线

## [2026-07-06] 用户关系（relation）新服务基建接入（部署/监控/CI）

**变更内容**: relation 独立 gRPC 微服务接入部署与可观测栈——docker-compose 服务块 + Dockerfile + prometheus 抓取 + grafana 告警 + CI workflow + `.env` 变量，完成 CLAUDE.md「服务拆分」14 项清单。
**影响范围**: 新增 `webook/relation/Dockerfile`、`.github/workflows/webook-relation-ci.yml`、`deploy/grafana/provisioning/alerting/webook-relation.yml`；改 `deploy/docker-compose.yaml` + `docker-compose.local.yaml`（webook-relation 服务/override）、`deploy/prometheus/prometheus.yml`（job）、8 份 `deploy/.env.*{,.example}`（`RELATION_IMAGE_TAG`/`RELATION_APP_ENV`）、6 兄弟 CI 的 `paths-ignore`（加 `webook/relation/**`）、`webook/relation/CLAUDE.md` 部署段。
**技术决策**: 镜像 interaction（同为纯 gRPC 内部服务）——HTTP `:8060` 仅 `/metrics`+`/health`，业务走 gRPC `:8061`，不入 nginx；告警用 gRPC 表达式（无 HTTP 业务流量）；metric 命名 `webook_*`，service 靠 `job` label 区分。无需改动：`deploy.sh`（服务名透传）、grafana 看板（`$service` 变量 / `label_values(up{job=~"webook-.*"})` 自动纳入）、prometheus 录制规则（无 cron/lock）、nginx（gRPC 经 etcd）。
**待办**: 仅剩前端（webook-fe：`api/relation.ts` + types + 关注按钮 3 态 + `/user/[id]` + profile 关注/粉丝 Tab + `blocklist` + 移动端）。**续接见 `prd/relation/HANDOVER.md`**。
**会话**: 260706-relation-用户关系系统
**发布**: 待上线

## [2026-07-06] 用户关系系统（relation）设计 + 后端全链路（进行中）

**变更内容**: 新增用户关系模块（关注/粉丝/拉黑）。完成需求+原型（`prd/relation/`：`relation.pen` + 5 PNG + PRD/ARCHITECTURE）+ 后端独立 gRPC 微服务 `webook/relation/` 的全链路业务逻辑（DAO/cache/repository/service/errs/domain/proto/gRPC server），32 条真库集成用例全绿。
**影响范围**: 新增 `webook/relation/**`（domain/dao/cache/repository/service/errs/grpc/consts/scripts/integration/config）+ `api/proto/relation/v1/relation.proto` + `api/gen/relation/v1/**`；改 `webook/worker/event→events`（对齐 core 复数）；`webook/CLAUDE.md`（端口分配铁律 + 转换命名多类型）；`prd/relation/**`。
**技术决策**: ① 独立 gRPC 服务（端口 8060/8061）——feed/chat 跨服务消费，放 core 会反向依赖网关；② 关注边 status 翻转 + 行锁 + GREATEST 计数（镜像 interaction）；③ 计数 cache-aside + 写后失效；④ 事件在 core 生产、gated on changed，relation 纯同步（对齐 interaction 边界铁律）；⑤ feed 后续独立服务，relation 只暴露关系查询 gRPC + `relation_events`。
**待办**: ioc+wire+main（服务可跑）→ core 接入（web 8 endpoint + client + 事件 + wire）→ 前端 → 14 项新服务基建。**续接见 `prd/relation/HANDOVER.md`**。
**会话**: 260706-relation-用户关系系统
**发布**: 待上线

## [2026-07-05] 外部凭据 env 变量改纯厂商命名（DEEPSEEK/KIMI/QIANFAN）

**变更内容**: `LLM_DEEPSEEK_API_KEY`/`LLM_KIMI_API_KEY`/`EMBEDDING_API_KEY` → `DEEPSEEK_API_KEY`/`KIMI_API_KEY`/`QIANFAN_API_KEY`。按「厂商凭据」命名（与能力无关，业界 `OPENAI_API_KEY` 风格），补掉 `EMBEDDING_API_KEY` 看不出厂商（百度千帆）的一致性缺口。
**影响范围**: core 5 份 + chat 4 份 yaml 的 `${ENV}` 占位；`deploy/docker-compose.yaml` 转发键；4 份 `deploy/.env.*.example` + `internal/config/.env.example` + `chat/config/.env.example`；`webook/CLAUDE.md` + `prd/config/config-architecture.md` §5/§9/§13/§14。无 `.go` 改动（应用读 yaml 键、不引用 env 名）。
**技术决策**: 纯厂商（vs 能力+厂商 `EMBEDDING_QIANFAN`）——key 属厂商、能力无关（deepseek 若出 embedding 同 key 通用），对齐 OpenAI/Anthropic SDK；厂商 token 用平台产品名（`QIANFAN` 对齐 base_url、非公司名 `BAIDU`，同 `KIMI` 非 `MOONSHOT`）。
**待办**: 部署者同步各环境真实 `.env`（`deploy/.env.<env>` + 本地 `<svc>/config/.env`）的键名，趁 key 轮换一并换。
**会话**: 260704-config-配置架构落地

## [2026-07-04] viperx `${ENV}` 展开改为解析后注入（消除 yaml 注入隐患）

**变更内容**: 原 `expandEnv` 在 **yaml 解析前**对原始字节做 `${NAME}` 文本替换——密钥值含 yaml 特殊字符（`#` / `: ` / 换行 / 引号 / `{[`）会破坏结构、静默取错值、甚至注入伪键。改为**解析后展开**：`yaml.Unmarshal` 出配置树 → `expandTree` 递归只对**已解析的字符串叶子**做 `${NAME}` 替换（map / slice 全走到，覆盖 `providers[]` 列表元素）→ `viper.MergeConfigMap` 塞回。展开值不再经 yaml 解析器 → **结构上无法注入**，密钥可含任意字符。保留：仅 `${NAME}`（裸 `$` 不碰）、未设置→空、单遍不二次展开。
**影响范围**: `pkg/viperx/viperx.go`（`expandEnv([]byte)`→`expandEnvValue(string)`+`expandTree(any)`，`LoadLocal` 走 `yaml.Unmarshal`+`MergeConfigMap`；`go.mod` 提升 `gopkg.in/yaml.v3` 为直接依赖）；`viperx_test.go` 加 `TestExpandTreeInjectionSafe`（恶意值含冒号/井号/换行/引号/伪键，验证原样落地、slice 展开、不注入结构）；`prd/config/config-architecture.md` §9 + `webook/CLAUDE.md` 同步。
**技术决策**: ①**解析后注入**是结构上免疫（vs 加引号只是打补丁、仍挡不住值内引号/换行）；②不用 viper 官方 `BindEnv`（整值覆盖、免疫更彻底）因它做不了嵌入式 `root:${MYSQL_PASS}@tcp` 且要逐键 boilerplate——留 L2 与 K8s Secret 一起上；③`os.ExpandEnv` 官方但连裸 `$var` 也展开会误伤含 `$` 的值，故仍用自造 `${NAME}`-only 正则（本就是它的更安全子集）。**残留**：DSN 内嵌密码含 `@`/`:` 坏 DSN 格式是正交语法约束（密码需 url-safe），非展开层能解。
**会话**: 260704-config-配置架构落地

## [2026-07-04] viperx 本地 `.env` 自读（补密钥出 git 后的本地运行体验）

**变更内容**: 密钥全走 `${ENV}` 出 git 后，裸 `go run` / `go test` 拿不到 LLM/embedding 凭据。给 `viperx.LoadLocal` 加 `loadDotEnv`（godotenv 解析）**自读「配置文件同目录」的 `.env`**（`config/local.yaml`→`config/.env`）——与 yaml 同目录、不依赖 CWD；**只填未设置的键**,优先级 真实环境变量（含 IDE Run Config）> `.env`；文件不存在直接跳过（prod/CI 无 `.env` 属正常）。**不做 per-env `.env.<env>` 选择**（KISS/YAGNI：本地基本只跑 local.yaml，dev/staging/prod 跑 docker 走 compose 转发密钥，本地按环境分密钥的需求不存在）。
**影响范围**: `pkg/viperx/viperx.go`（+ `viperx_test.go`：`TestLoadDotEnv` 3 组、注入安全）；`go.mod` 加 `github.com/joho/godotenv v1.5.1`；`.gitignore` 忽略 `.env` / `.env.*`（track `.env.example` / `.env.*.example` 模板）；新增 `webook/internal/config/.env.example` 模板；`prd/config/config-architecture.md` §9 + `webook/CLAUDE.md` 环境说明同步。
**技术决策**: ①**用 godotenv 而非自造解析**（初版手写 ~30 行，复盘：`#` 行内注释 / 引号 / 转义等边角自造易出 bug——如未加引号值的行内注释会被当成值，客观不如 godotenv 可靠；godotenv 零传递依赖、MIT、事实标准，仅本地启动跑一次，破例引它换掉自造解析器值得）；②`godotenv.Load` 不覆盖已存在 env → 真实 env 优先，prod 天然安全（无 `.env` 文件 + 有也不覆盖）；③精确名 `.env` gitignore（deploy 那套叫 `.env.dev` 等，互不干扰）。
**会话**: 260704-config-配置架构落地

## [2026-07-04] 配置架构标准化落地（全仓迁移 + 设计重定稿）

**变更内容**: 按重定稿的 `prd/config/config-architecture.md` 全仓迁移：域分组 `server`/`client`/`data` + 叶子键 snake_case + 时间值 duration；`grpcx.ServerConfig` Port→Addr（破坏性）+ `ClientConfig` 扩展（keep_alive/retry/timeout/msg size，从类型化键组 grpc service config）；超时治理落地（gRPC unary 拦截器 5s + HTTP 软超时中间件 15s，chat SSE `/chat` 豁免、**core `/article/polish`（LLM 60s）+ `/search/article`（embedding）豁免**避免慢 AI 调用被 15s 切、streaming 天然豁免、client timeout opt-in）；viperx `${ENV}` 占位展开 + 热更 override（远程子集 `viper.Set` 绕开 file>kvstore）+ `webook_config_reload_total` 指标；六服务 ioc 按段 `UnmarshalKey`、logger 改 yaml 驱动（`development` 选 base + `level` 字符串枚举）、删 grpcClientConfig 隐式 target 推导（每下游一 Provider 显式读）；密钥全环境 `${ENV}` 出 git（MYSQL_PASS/REDIS_PASS/LLM_*/EMBEDDING）+ docker-compose 转发 + `.env.*.example` 登记；Grafana 加远程配置热更失败告警（`webook_config_reload_total{status="error"}` 跨服务多序列，`increase[10m]>3` 降级告警 severity=high）。
**影响范围**: `pkg/{viperx,grpcx,ginx/middleware/timeout,ginx/middleware/accesslog}`（含单测）；业务段 config struct `pkg/llm.ProviderConfig` + `internal/service/embedding/{openai,ollama}.go` 补 snake tag、`migrator/ioc/engines.go` + `internal/ioc/migrator_sdk.go` + migrator 集成测试改取键；六服务 `*/ioc/*.go` + `*/main.go` + `*/config/*.yaml`（30 份，业务段键一并 snake）；**集成测试 DI 同步迁移**（`{comment,interaction,internal}/integration/setup/{db,redis}.go`、chat/migrator integration 及 `article_reader_v1_test.go` 共 15 文件，`mysql.dsn`→`data.mysql.dsn`、`redis`→`data.redis`——上轮域分组漏改，ship review 补齐）；`deploy/docker-compose.yaml` + `deploy/.env.*.example` + `deploy/grafana/provisioning/alerting/webook-config.yml`（新增）；`prd/config/config-architecture.md` 重定稿。
**ship review 修复**: accesslog builder 去 `_ = err`（改 log，热更回调不 panic）+ 修陈旧 godoc（`web.logger`→`server.http.access_log`）；grpcx client 补 `_ "grpc/health"` blank import（`health_check:true` 才真生效）+ `GRPCRetry.policy()` 对缺省字段就地兜底（部分 retry 配置不再让 service config 非法拨号失败）；grpcx server 亚秒 TTL 向上取 1s（避免 `Grant(0)` 永不过期）；otel struct `Endpoint` 死 `yaml` tag 统一 `mapstructure`（4 服务）。**遗留 Suggestion（未改）**：otel 默认值三服务设满/三服务全空——设计取舍（wiring 键该不该有代码默认），无功能影响待定。
**技术决策**: ①**去 Kratos 化**（设计以 grpc-go 语义自证）；②**彻底不校验**（用户定）：无 Bootstrap/MustLoad/Validate，配置错误靠消费点自然失败；③**显式配置 + 代码兜底**（用户二次确认）：yaml 显式写调参键、代码默认仅 fallback；④**类型各归其位**（grpcx 自带 Server/ClientConfig，叶子段 ioc 内联，不建 configx）；⑤**全叶子键 snake_case,无业务段例外**（复盘：起初图省事让 `llm`/`embedding`/`ollama`/`migrator.*` 留 camelCase,后统一收口——给 `pkg/llm.ProviderConfig`、`embedding.Config`/`OllamaConfig` 补 `mapstructure:"snake_case"` tag,migrator pipeline 直接取键改 snake;因 viper 大小写不敏感匹配下 snake 带下划线**必须有 tag** 才绑得上,无 tag 只能绑 camelCase）；⑥client timeout opt-in（非幂等写不能在服务端提交前放弃）；⑦顺带修 task-1 埋的 `ConfigFileUsed` 回归。
**待办**: **上线前每环境实际启动跑通关键路径**（gRPC 拨号/连库/发 kafka/SSE 不误杀）——本机无 etcd/mysql 未真跑，仅 build + yaml 冒烟通过；泄露的 LLM/embedding key 供应商侧轮换（deepseek `sk-405db…` / kimi `sk-KURYx…` / 百度千帆 `bce-v3/ALTAK-WN6t…`）；viper override 的 etcd 集成测试；`.env.<env>` 实际值部署者按 example 同步。
**会话**: 260704-config-配置架构落地
**发布**: 

## [2026-07-03] 配置架构设计文档（标准版定稿）

**变更内容**: 参考 Kratos 设计全仓配置架构：`server`/`client`/`data` 域分组、叶子键 snake_case、时间值 duration 字符串、每服务集中 Bootstrap struct + 启动 Validate()（fail-fast）、密钥 `${ENV}` 占位出 git、config reload 指标、yaml 注释规范；gRPC client 删除隐式 target 推导改显式必填 + 依赖清单双向校验；含存量迁移映射（P1 结构迁移 / P2 超时治理）。
**影响范围**: 新增 `prd/config/config-architecture.md`（纯设计文档，未动代码）。
**技术决策**: ①取 Kratos 层级与 fail-fast，不取 conf.proto（脱离其 config loader 生态形似神不似）；②显式优于隐式：禁止代码为缺失配置静默兜底，仅允许登记过的功能开关式缺省；③密钥两级策略（外部真实凭据全环境 env 化、本机 docker 自建密码 local 可明文）；④发现 viper config file 层级高于 remote kvstore，现行「etcd put 全量 yaml」热更思路可能同名键不生效，待 P1 实测。
**待办**: P1 六服务结构迁移（worker→interaction→comment→chat→migrator→core）；git 历史已泄露的 LLM/embedding apiKey 供应商侧轮换；viper 本地/远程优先级实测回填；P2 超时治理另开会话评审阈值。
**会话**: 260703-config-配置架构设计
**发布**: 

## [2026-07-03] 评论删除改整楼级联 + 移除占位机制

**变更内容**: 删一级评论 → 整楼级联软删（自身 + 全部 root_id 命中回复，一条 UPDATE）；删楼内回复 → 只删自身 + 楼根 reply_cnt−1（子回复保留，楼内扁平不级联）。前端确认框对有回复的一级评论提示「删除后，N 条回复也将一并删除」。占位机制（`deleted` 列 / pb 字段 / domain / VO / 前端「该评论已删除」渲染分支）整体移除。
**影响范围**: `comment/repository/dao/comment.go`（Delete 重写 + Comment 模型删 Deleted + Count 去 deleted 过滤）、`comment/repository/comment.go`（Delete 复用 DAO 返回行清缓存，删前置 FindById）、`comment/domain`、`comment/grpc`、`api/proto/comment/v1` + 重生成 pb、`internal/web/comment.go`（VO）、`comment/scripts/comment.sql`、前端 `types/comment.ts`、`CommentItem.tsx`、`CommentSection.tsx`（删楼后清 repliesMap/expanded）、集成测试 2 个新用例替换占位用例；`prd/comment/` 同步（PRD.md / ARCHITECTURE.md 删除语义 + `comment.pen` 移除占位楼、新增删除确认 frame + 重导出 desktop / requirements / delete-confirm PNG）。
**技术决策**: ①删除前置提示优于删除后留占位尸体，讨论串完整性靠提示让用户知情，不靠保留空壳；②DAO Delete 返回命中行，删除链路 SELECT 5→3 次（去掉 repo 前置 FindById 与子回复 COUNT）；③学习项目无存量兼容负担，占位机制整体拆除不留双模式。
**待办**: 已有环境 comment 表需先清占位行再删列：`DELETE FROM comment WHERE deleted = 1` + `ALTER TABLE comment DROP COLUMN deleted`（AutoMigrate 不删列，占位行不清会变成空内容评论展示）；级联删除后 interaction(biz="comment") 点赞数据成孤儿（既有问题，后续统一清理）。
**会话**: 260703-comment-删除级联优化
**发布**: 

## [2026-07-02] deploy.sh 支持按单服务操作

**变更内容**: `up`/`build`/`pull`/`status`/`logs` 接受可选 `[service...]` 参数（可传多个，`"${@:3}"` 全量透传；指定服务 up 自动带 depends_on 链，local 先 build）；新增 `stop [service...]`（停容器保留）与 `rm <service...>`（`compose rm -sf`，停+移除容器、volume 保留）；`down`/`nuke` 拦截误传服务名（防"想停一个停掉全站"）。
**影响范围**: `deploy/deploy.sh`（用法注释 / ACTION 白名单 / case 分支）。
**会话**: 260702-deploy-cron告警NoData拆分
**发布**: （脚本同步到服务器即生效）

## [2026-07-02] Go Dockerfile 统一 GOPROXY，修国内构建拉包失败

**变更内容**: 6 个 Go 服务 Dockerfile 的 `go mod download` 前置 `ENV GOPROXY=https://goproxy.cn,direct`——VM 上 `./deploy.sh local` 构建直连 proxy.golang.org 被拒（1044s 超时）。
**影响范围**: `webook/{internal,chat,comment,interaction,worker,migrator}/Dockerfile` 各一行。
**技术决策**: 对齐 fe Dockerfile 写死 npmmirror 的先例；goproxy.cn 全球 CDN，CI 同源（顺带消除 proxy.golang.org 偶发抖动）。
**会话**: 260702-deploy-cron告警NoData拆分
**发布**: （随各服务下次镜像构建生效）

## [2026-07-02] Cron 告警 NoData 语义拆分：absent 哨兵接管指标缺失

**变更内容**: `webook-cron-stale` 告警在 DatasourceNoData 状态下 summary 模板（`$values.B.Value`）展开失败、通知输出模板原文；将其 `noDataState` 改为 OK，新增 ①b `webook-cron-metrics-absent`（`absent(webook:cron_last_success_age:seconds{task=~"ranking_.*"})`，静态文案不依赖 `$labels/$values`）单独建模"指标缺失"。
**影响范围**: `deploy/grafana/provisioning/alerting/webook-jobs.yml`（7→8 条规则，① ② 文案随 worker 拆分矫正：可能原因补 gRPC 派发失败、panic 排查指向 worker 日志）；`deploy/grafana/examples/alerting/rules-recording-example.yml`（教学示例移除 NoData+$values 地雷写法并加警示注释）。全仓扫描确认无其余同款硬伤（6 条 up 告警 NoData 时仅 $labels 渲染空串、不炸模板，维持现状）。
**技术决策**: NoData 不是噪音——对应"worker 任务从未成功/未被抓取"的真实故障态（本次由旧 core 镜像不实现 RankingJobService、worker 调用持续 Unimplemented 触发），拆独立告警比在模板里判空补丁语义更清晰；`for: 5m` 容忍冷启动（任务 30s 节奏，首成功后序列即出现）。其余 6 个服务的 up 告警 NoData 模板只引用 `$labels`（缺失渲染空串不报错），不在本次范围。
**待办**: 服务器同步该文件 + reload Grafana alerting provisioning 后实机验证（停 worker ≥5min 应触发 ①b 且文案可读）。
**会话**: 260702-deploy-cron告警NoData拆分
**发布**: （随下次部署同步 deploy/ 目录生效，不依赖服务发版）

## [2026-07-01] ginx 响应/错误层重设计：code≡HTTP status + wrapper 精简

**变更内容**: 重设计 `pkg/ginx` 响应契约与 wrapper 一族，让响应一致、易排查、handler 更简单（完整标准版，不兼容旧写法）。
**影响范围**:
- **契约**: `body.code ≡ HTTP status` 全链路——成功框架填 `code=200` + `msg` 空补 "OK"（此前成功 `code=0` 与错误 `code=4xx/5xx` 不一致）；错误经 `WriteError` 单点出口带 code/reason/msg/metadata；bind 失败也走 WriteError（400 + `reason=BAD_REQUEST` + 日志）。
- **wrapper 从 4 变 2**: 只留 `Wrap`（无请求体）/ `WrapReq[Req]`（绑 JSON）；鉴权解耦为访问器 `MustClaims[C](ctx)`（受保护路由，缺失 panic→500 fail-loud）/ `Claims[C](ctx)`（可选登录），删掉 `WrapClaims`/`WrapReqClaims` 组合变体 + `claimsOf`/`bindBadRequest`。包级 `ginx.UserKey` 各服务启动设一次（core/chat `ioc/web.go`）。
- **命名构造器**: `ginx.Ok(data)`/`OkWith(msg,data)`/`BadRequest`/`Unauthorized`/`Forbidden`/`NotFound`/`Conflict`/`TooManyRequests`/`Internal`——状态码由函数名写进 `Result.Code`，调用方只给 msg/data，`respond` 按 `Result.Code` 作 HTTP status（`return ginx.NotFound("x"), nil`）。无 reason（需 reason 监控/分支的业务错误仍用 errs sentinel）。
- **迁移**: core + chat 共 24 个 claims handler 去掉 `uc` 参数、体内 `MustClaims` 取登录态、路由改 `Wrap`/`WrapReq`；SSE handler（chat SendMessage/ResumeStream）也改用 `MustClaims/Claims`。
- **前端**: 一律 HTTP status 驱动——`await` resolve 即成功、错误进 `catch`（`getErrorMessage`/`getErrorReason`）；删 12 文件 ~30 处 `res.data.code === 0` in-band 判断，折成 happy-path + catch（含 useConversations/useChat 哨兵改 throw、article/list modal onOk 补 catch）。
- **测试**: `pkg/ginx/wrapper_test.go` 合并原 `wrapper_errs_test.go`+`wrapper_variants_test.go` 为单文件（遵守新规则「一源一测试」）；web 单测加 `ginx.UserKey` init + 成功断言 `code:0→200`；integration 加泛型 `assertResult`（成功自动补 200/OK）。
**技术决策**: 保留类型化 wrapper（handler 零样板）但把鉴权（横切）解耦成 ctx 访问器 → wrapper 面永远 2 个、按「为什么」而非「在哪」组合，新增输入轴只加访问器不炸变体（前瞻）。成功 code 由框架填而非 handler，杜绝 `code=0`。
**待办**: 无（后端 `go test ./...` 全绿、前端 tsc/eslint 净）。
**会话**: 260630-前端-错误处理对齐
**发布**: （随 core / chat / fe 下次发版）

## [2026-06-30] 错误模型重构：Kratos 风格 reason 双级标识 + 前后端对齐

**变更内容**: 引入业务原因码 `reason`，把「错误身份」从 HTTP `code`/`msg` 解耦——`code` 仍=HTTP status（粗分类 + 前端 catch 触发），`reason` 给业务精确分支 + 监控，`msg` 回归纯展示。前端 catch 块对齐成展示后端真实 `msg`。
**影响范围**:
- `pkg/errs.Error`: +`Reason` 字段 + `WithReason` builder；`Is` 改**优先按 reason**（双方有 reason 比 reason，否则回退 Code+Message → 迁移期新旧 sentinel 混用都正确）。
- `pkg/ginx.Result`: +`Reason`(omitempty) + `Metadata`(omitempty)；`WriteError` 业务错误带出 reason+metadata。
- 跨 gRPC: `GRPCStatus`/`FromError` 用 `errdetails.ErrorInfo` 带 reason+metadata（顺带修跨服务丢 Metadata），worker/core/chat → interaction 业务错误保真；`FromError` 对**无 ErrorInfo 的传输层错误**（`Unavailable`/`DeadlineExceeded`/`Canceled`，如下游未启动→`name resolver error: produced zero addresses`）换友好文案 + reason（`SERVICE_UNAVAILABLE`/`SERVICE_TIMEOUT`/`REQUEST_CANCELED`），原始内部细节留 `cause` 不外泄；**修 HTTP code 跨 gRPC 有损退化**——`GRPCStatus` 把原始 HTTP code 塞进 details metadata（`_http`），`FromError` 读回精确还原并剥掉内部 key，任意码（如 comment 敏感词 `422`）不再退化成 `500`（此前会污染 5xx 监控）。
- sentinel: 7 个 `*/errs` 共 **55 个 reason**（SCREAMING_SNAKE），双 guard 测试（唯一性 + 全 sentinel 必须有 reason）。
- 监控: `webook_http_requests_total` +`reason` label（仅错误路径打、低基数）；录制规则 `webook:http_biz_errors:rate5m`(`sum by job,reason`) + `webook-overview` 看板「业务错误（按 reason）」面板（`topk(10) by reason`）；告警按 reason 多序列精确触发——core 加「限流家族 `*_RATE_LIMITED`」+「登录失败激增（撞库）`USER_OR_PASSWORD_INVALID`」，chat 加「限流 `*_RATE_LIMITED`」（migrator 的 5xx 类 reason 已被现有 5xx 告警覆盖，不重复）。
- 前端: `types/common.ts` `Result` +`reason/metadata`；`utils/apiError.ts` `getErrorMessage`/`getErrorReason`；user/article/search/chat 等 view/组件的 catch 块由「吞后端错误弹通用文案」改为 `getErrorMessage(e, 兜底)` 展示后端 `msg`。
- **401 刷新判定改为 reason 驱动**（修「登录密码错误显示『系统错误』」bug）：原拦截器对所有 401 都走 token 刷新，把登录的业务 401 也当过期吞掉（refresh 失败 Error 盖原始 401 → 显示「系统错误」+ 误重定向）。改为：后端 `pkg/jwtx` 中间件 401 带 reason（`ACCESS_TOKEN_EXPIRED` 可刷新 / `TOKEN_INVALID` 去登录，`Parse`→`parseWithReason` 区分过期）；前端 `api/request.ts` 拦截器据 reason 三分支——过期才静默刷新、无效去登录、业务 401（如 `USER_OR_PASSWORD_INVALID`）原样透传给 handler 显示后端 msg。按「为什么」而非「在哪/状态码」决策，对新端点天然健壮。
- 测试同步: `internal/web` + `internal/integration` 的 user/wechat handler 断言补 reason（sentinel 带 reason 后的陈旧断言修复）。
- 设计文档: `prd/error-model/error-model-design.md`。
**技术决策**: 不全量上 Kratos，只增量扩 `errs`/`ginx`/`grpcx`；reason 各服务 `*/errs` 自治但自动化 guard 强约束（不靠人肉 review）；HTTP/gRPC 共用同一套 reason 枚举；`msg` 绝不当 metrics label（高基数炸 Prometheus）。Phase 6（收紧 `Is` 去 Code+Message 回退）暂缓——回退是无害安全网。前端 `views/chat/index.tsx` 3 个 `.catch(()=>哨兵)` 布尔流 handler 暂留（重构控制流有风险、对话 CRUD 失败业务价值低）。
**待办**: Phase 6 收紧 `Is`（待全量 reason 覆盖稳定后）；前端 chat/index 哨兵流 handler 视需对齐；reason 告警阈值（限流 0.5/s·10m、撞库 2/s·5m）是起步值，按真实流量调。
**会话**: 260630-错误模型-reason 双级标识
**发布**: （随 core / interaction / chat / worker / fe 下次发版）

## [2026-06-30] 多服务架构审查收尾：并发/锁/契约/批量 RPC 硬化

**变更内容**: 对 interaction 拆分 + webook-worker 抽离的全部改动做架构审查，修复 2 Critical + 9 Important + 多个 Suggestion。
**影响范围**:
- **interaction**: ① UpsertLike/UpsertCollect 事务内读旧状态加 `FOR UPDATE` 行锁（修并发同用户点赞/收藏计数翻倍）；② 建表 `created_at/updated_at` 改 `NOT NULL DEFAULT 0`（soft_delete 一度加了又**撤回**——见技术决策/事故）；③ `FindByBizIds` 去 fire-and-forget goroutine，改同步回填（复用 caller ctx）；④ 新增 `BatchIncrReadCount` RPC（proto + 4 层实现）；⑤ 抽 `errs` sentinel 包；⑥ 去 `FindUserInteraction` 多余 `Order`。
- **worker**: ① cron 锁显式 `WithLockTTL(30s)`（原吃 2s 默认值，分钟级任务有 split-brain 风险）+ grafana 加 `watchdog_lost` 告警；② 消费者关停有界排空再 `cleanup`；③ 消费 read 事件按 `(biz,bizId)` 聚合走 `BatchIncrReadCount`（取代逐条 N+1，并修批内部分失败整批重投的重复计数）；④ 删死 `etcd.path/type` 配置（worker 静态配置不接热更）。
- **契约/共享**: ① ranking `dimension` 由两端字符串字面量改 proto `enum`（编译期防漂移）；② 新增 `worker/event/contract_test.go` 守护 `InteractionEvent` 跨服务不漂移；③ `pkg/cronx` 默认锁 TTL `2s→30s`（对齐自身文档）；④ `pkg/pool` 补「为何不用 x/sync」说明。
- **core 瘦身**: 删残留消费者侧 kafka 配置（`KafkaConfig` 字段 + 5 份 yaml）+ 死 producer 导出（NoopProducer/Async/未用构造器）。
- **文档/CI**: 补 `interaction/CLAUDE.md` + `worker/CLAUDE.md`，更新多服务布局约定；修 chat/migrator CI `paths` 缺 `.github/workflows/` 前缀。
**技术决策**: 事件契约维持「两端各自定义」但补契约测试兜底（不回头建 eventbus）；worker 维持静态配置；dimension 走 proto enum（gRPC 契约即共享真相，区别于 Kafka 事件的 topic+JSON）。幂等修复选悲观行锁（最小改动且贴合现有「读-比-翻」结构）。interaction **不加 gorm soft_delete**：无删除路径，且软删会给所有 SELECT 注入 `deleted_at=0`，把 AutoMigrate 加列后未回填的既有 NULL 行全部过滤掉（实测导致点赞/收藏/计数查出全 0），故撤回审查「补 soft_delete」那条 + 加 integration 回归测试守护。
**待办**: `services-overview.json` 的 `$service` 模板变量未定义（pre-existing，面板靠 match-all 仍渲染，需在 Grafana 内预览后补）；`pkg/pool` `TestPool_Introspection` 在本机 flaky（in-flight 计数竞态，与本次改动无关，doc-only 改动不影响）。
**会话**: 260630-多服务-架构审查修复
**发布**: （随 core / interaction / worker 下次发版）

## [2026-06-29] webook-worker 调度器服务抽离（cron + 消费者）

**变更内容**：新增独立调度器服务 `webook/worker/`（端口 8050，无 DB），收拢所有异步/定时：cron 定时任务 + Kafka 消费者，**全部经 gRPC 派发给业务服务，自身零业务数据/逻辑**（参考 `bolee-task` 的 XxlJob+StreamListener 模式）。
**影响范围**：
- **proto**：新增 `ranking/v1` = `RankingJobService`(Recompute{dim,date}/Archive{date})——core 实现、worker 触发。
- **webook/worker/**：`job/`(cron specs → 调 core RankingJobService gRPC)、`consumer/`(read 事件 → 调 interaction gRPC，自管连接无限重连)、`event/`(自有 InteractionEvent；契约=topic+JSON，不共享代码)、`ioc/`(redis 仅 cron 锁 / kafka 消费 / etcd 发现 / gRPC client core+interaction / cron) + main/wire + 5 config + Dockerfile/Makefile。
- **core 瘦身**：加 `internal/grpc/ranking_job.go`(RankingJobServer → 转调 in-process RankingService)；**删进程内 cron**（ioc/cron.go、lock.go + wire cron 全套）；**删 read 消费者**（App.Consumer + kafka 只剩生产侧）；删 `internal/worker/`、cron 集成测试。ranking repo/dao/cache/service/Page/Click **全留 core**（数据/逻辑不迁）。
- **部署**：prometheus job `webook-worker:8050`、grafana 告警 `webook-worker.yml`(up/cron 失败/goroutines)、docker-compose 服务 + healthcheck、8 份 `.env*` 加 `WORKER_IMAGE_TAG/APP_ENV`、CI `webook-worker-ci.yml` + 5 兄弟 paths-ignore 互斥。
**技术决策**：调度器 ≠ 业务服务——worker 只调度/派发不持数据，业务逻辑留各 owner 服务（ranking 留 core）；不建 `pkg/eventbus` 共享抽象，通用 plumbing 复用 `pkg/saramax`，事件契约靠 topic+JSON（broker 即边界，两端各自定义）。cron 锁用 redislockx（多副本单跑）。
**待办**：worker job/consumer 单测（mock gRPC client）+ core RankingJobServer 单测；启动解耦 producer 侧（core InitSaramaSyncProducer 仍阻塞 ~63s）。
**会话**：260629-worker-异步任务收拢
**发布**：（随 core / worker 下次发版）

## [2026-06-29] interaction 拆分为独立 gRPC 微服务

**变更内容**：互动（点赞/收藏/浏览/计数）从 core 进程内 service 抽成独立后端微服务 `webook/interaction/`（纯 gRPC，HTTP :8040 仅 metrics/health、gRPC :8041），与 comment 同构；core 转薄 gRPC 客户端经 etcd 调用，chat 的互动调用从 core 重指 webook-interaction。
**影响范围**：
- 契约：`api/proto/interaction/v1/interaction.proto` 补 9 个 RPC（Like/CancelLike/Collect/CancelCollect/IncrViewCount + GetInteraction/GetUserState/BatchGetInteractions/GetUserLiked），全 `(biz, bizId)`；既有 GetHotBizIds/GetCollectedBizIds 不变。
- 新服务 `webook/interaction/`：domain/consts/dao(+init_table)/cache(+lua)/repository/service/grpc/ioc/main/wire + 5 份 config + Dockerfile/Makefile/scripts/interaction.sql + bufconn 集成测试（覆盖全 11 RPC）。
- core：`internal/service/interaction.go` 加 `GRPCInteractionService` 适配器（生产路径）；ranking 装饰器留 core、Kafka 读计数 producer/consumer 留 `internal/`（路 A：同库直写，待 worker 收拢）；删 `internal/grpc/interaction_server.go`。
- chat：`chat/ioc/grpc.go` 抽共享 metrics builder + 加 `InteractionConn`；5 份 config 加 `grpc.client.webook-interaction`。
- 部署：prometheus job（+local）、grafana 告警 `webook-interaction.yml`、docker-compose 服务 + healthcheck、8 份 `.env*` 加 `INTERACTION_IMAGE_TAG/APP_ENV`、CI `webook-interaction-ci.yml`（+ 4 个兄弟 workflow paths-ignore 互斥）。
- 修复既有幂等 bug：`UpsertLike`/`UpsertCollect` 改「读旧状态比对翻转」（原 `RowsAffected==0` 判幂等因 `updated_at` 恒变失效 → 重复点赞/收藏计数虚高），interaction + core 两份 dao 同步修。
**技术决策**：与 comment 同库不分库 → 无数据迁移；ranking 依赖 core `RankingRepository` 故装饰器留 core；异步/消息统一方向是独立 `webook-worker` 服务（本次不拆，read 计数消费者暂留 `internal/`，gRPC 边界靠真拆进程时收口）；`allowedBiz` 白名单留 core 网关侧，interaction gRPC 只做非空校验。
**待办**：worker 结构收拢（`internal/job` + `internal/events` → 干净的 `internal/worker/`）；core 自身集成测试预存失败（webook_test 数据隔离，与本次无关）待单独清理。
**会话**：260629-interaction-拆分独立服务
**发布**：（随 core / interaction 下次发版）

## [2026-06-29] interaction 点赞/互动端点泛化为 (biz, bizId)

**变更内容**：interaction HTTP 端点（like/collect/detail/state/view）从写死 article 改为通用 `(biz, bizId)` + biz 白名单（article/comment）；评论点赞回归 interaction——删 core `/comment/like`，前端评论点赞改调 `/interaction/like {biz:"comment"}`。
**影响范围**：后端 `internal/web/interaction.go`（泛化 + 白名单 + `ErrInvalidBiz`）、`internal/web/comment.go`（删 `Like`/`commentLikeReq`）、`comment_test.go`（移除 2 个 Like 测试）、新增 `interaction_test.go`（泛型 like + 白名单覆盖）；前端 `api/interaction.ts`（泛型 `like`/`collect` + 文章便捷包装）、`api/comment.ts`（删 `likeComment`）、`types/{comment,index}.ts`（删 `CommentLikeReq`）、`components/comment/CommentSection.tsx`（改调 interaction）。
**技术决策**：点赞是交互行为，统一到 interaction，撤销原“`/comment/like` 不动 interaction 契约”的取舍。当前阶段只做契约泛化（service 仍在 core 进程内），为后续把 interaction 拆成独立 gRPC 服务铺路；文章前端用 `*Article` 包装器吸收 biz，调用点不变。
**待办**：interaction 独立服务化（proto 补写 RPC → 拆包 → 独立部署 → core 转薄 gRPC 客户端 → 数据走 migrator 迁移）。
**会话**：260628-comment-评论功能
**发布**：（随 core 下次发版）

## [2026-06-29] comment 评论者建模收敛为 user_id（去本地 User 实体）

**变更内容**：comment 服务不再定义本地 `User{Id,Name}`，评论者仅存 `user_id`；昵称由 core 经 `userSvc.FindByIds` 批量解析填入对外 VO（与 likeCnt/liked 同一聚合层），并删掉 core VO 转换里恒空的 name 死兜底。
**影响范围**：`api/proto/comment/v1/comment.proto`（Comment/Create/Delete 字段统一 `user_id`，删 `message User`）+ regen pb；`comment/{domain,grpc,repository,service}`；core `internal/web/comment.go`；comment grpc/service/integration + core comment 单测。
**技术决策**：微服务不持有不拥有的用户展示数据（避免恒空字段的伪实体），不跨服务 import core `domain.User`（保服务边界）；昵称解析单一真相源在 core/user。注：interaction/chat proto 仍用 `uid`，跨 proto 命名统一留后续。
**待办**：comment 集成测试需起 MySQL/Redis 验证；`prd/comment` 文档若引用旧 `User` message 需同步。
**会话**：260628-comment-评论功能
**发布**：（随 comment 服务首次打 tag）

## [2026-06-29] migrator 端口移出业务段：8030 → 8200

**变更内容**：webook-migrator HTTP 端口 8030 → 8200，移出 80xx 业务序列（core 8010 / chat 8020 / comment 8030 / …），划入 82xx 运维控制台段，业务服务再增也不与 migrator 撞端口。
**影响范围**：`migrator/config/{local,dev,staging,prod,test}.yaml` http.addr + `migrator/main.go` 兜底；`deploy/`：`prometheus.yml` 抓取 target、`nginx/conf.d/default.conf` upstream、`docker-compose.yaml` healthcheck、`docker-compose.local.yaml` 端口映射、`.env.local(.example)` MIGRATOR_HOST_PORT。
**技术决策**：comment 占用业务序列的 8030，migrator 作运维控制台让出序列；选 8200 与 80xx(业务) / 88xx(otel) / 9xxx(exporter) 分段，心智清晰。
**待办**：`migrator/README.md` + `prd/migrator/**` 约 129 处 `:8030` 文档待同步。
**会话**：260628-comment-评论功能
**发布**：（随 migrator 下次打 tag）

## [2026-06-28] 评论功能全链路（盖楼无限嵌套 + 敏感词 + 点赞复用 interaction）

**变更内容**：新增评论功能端到端——①独立 gRPC 微服务 `webook/comment/`（CommentService：Create/List/BatchGet/GetReplies/Delete/Count），盖楼无限嵌套用 `root_id`(一级=0)+`pid`(自关联 FK)，敏感词本地 DFA `pkg/sensitive`；②core 作 HTTP 网关 `internal/web/comment.go`（list/replies/create/delete/like），经 etcd 解析调 comment gRPC，并聚合内部 interaction(biz="comment") 的 likeCnt/liked 填入 VO；点赞复用 interaction 数据层（新增 core `/comment/like` + interaction 批量 `FindUserLiked` 避免列表 N+1）；③前端 `api/comment.ts`+`types/comment.ts`+`CommentSection/CommentItem/CommentEditor` 挂载文章详情页（hot/new 排序、楼内回复懒加载、乐观点赞、删除本人评论）。

**影响范围**：新增 `webook/comment/`（全分层 + 33 测试）、`webook/pkg/sensitive/`、`webook/api/{proto,gen}/comment/`；改 core `internal/{web/comment.go,ioc/grpc.go,ioc/web.go,wire.go}` + interaction service/repo/dao 加 `FindUserLiked`/`FindLikedBizIds`；前端 `webook-fe/{api,types,components/comment,views/article/read.tsx}`；部署监控按「新增服务 14 类」同步（5 yaml/prometheus/docker-compose/8 .env/CI/Grafana 告警）。

**技术决策**：①点赞落 interaction(biz="comment") 不自建点赞表，core `/comment/like` 内部调 interaction service（不动 `/interaction/like` 的 article 契约）；②列表 liked 走批量 `FindUserLiked` map 而非 N+1；③最热由 core 聚合 interaction 计数内存 top N（comment 不存热度）；④core 同进程既是 gRPC server 又是 client，抽 `InitGRPCMetrics` 单例避免 `webook_grpc_requests_*` 重复注册 panic；⑤评论 list/replies 公开可读 + 登录态可选（中间件 `OptionalPaths`）有 token 才填 liked；⑥comment 纯 gRPC 后端不进 nginx，前端→core HTTP→gRPC(etcd)；⑦删除双模式：有子回复→`deleted` 占位（保留行清空内容，前端渲染「该评论已删除」、子树仍在，`Count` 不计），无子回复→`deleted_at` 软删消失；⑧回复 P0 扁平单层展示（每条带 `pid`，树形递归缩进 + @提及留 P1）；⑨删除走 `App.useApp().modal.confirm`（项目约定，避免 per-item `Popconfirm` 触发 antd CSS-in-JS 卸载告警）；⑩评论者昵称：comment 只存 uid，core 聚合时批量 `userSvc.FindByIds` 解析 uid→昵称填 VO（新增 user 服务 `FindByIds`）；⑪`reply_cnt` 维护在楼根（写/删回复增减 root_id），一级评论=整楼回复数，对齐扁平「展开 N 条回复」与展开实际条数。

**待办（P1）**：①proto `reply_preview` 已声明但 service 未编排（一级评论默认带前 N 回复预览），P0 走 `GetReplies` 懒加载；②回复树形递归缩进 + @提及高亮（P0 扁平单层，`pid` 已存可前端重建树）；③comment mock 部分未登记 `mk/mock.mk`。

**会话**：260628-comment-评论功能
**发布**：（未上线，待 comment 服务首次打 tag）

## [2026-06-25] gRPC 拦截器全家 + metrics 接入 + Grafana 看板

**变更内容**：`pkg/grpcx/interceptor` 铺平整套 gRPC 拦截器——`errconv`（`*errs.Error`↔status 双向转换，无配置故用纯函数 `UnaryServerInterceptor/UnaryClientInterceptor`）、`ratelimit`（全局/服务/方法三档，「方法>服务>全局」最具体优先）、`metrics`（`PrometheusBuilder` 开关式 `WithCounter/Histogram/Summary/InFlight`）、`logging`（method/耗时/peer/code + panic recover 不泄漏）、`circuitbreaker`（`go-kratos/aegis` 自适应熔断 fail-fast）、`tracing`（OTel span 注入/透传，tracer/propagator 可注入）；`peer` 工具由「空结构体嵌入」重构为包级函数 `PeerName/PeerIp`（修 IPv6 改 `net.SplitHostPort`）。五个 builder 方法名统一 `BuildUnaryServer/BuildUnaryClient`。metrics 接入 core(server)/chat(client) 的 gRPC 拦截链、经现有 gin `/metrics` 暴露；新增 Grafana `webook-grpc` 看板。
**影响范围**：新增 `pkg/grpcx/interceptor/{errconv,ratelimit,metrics,logging,circuitbreaker,tracing}` + `peer.go`（7 包 100% 测试覆盖 + metrics/logging/ratelimit benchmark）；改 `internal/ioc/grpc.go`、`chat/ioc/grpc.go`（`ChainUnaryInterceptor`，metrics 在外层）；新增 `deploy/grafana/provisioning/dashboards/webook-grpc.json`；`go.mod` 加 `go-kratos/aegis`；顺带 `prometheus.local.yml` 增 otel-collector/zipkin 抓取。
**技术决策**：①无配置的 errconv 用纯函数、有依赖/开关的用 builder（构造机制按配置复杂度匹配）；②metrics 标签 type/service/method/code/peer，但高基数的 code/peer 只放无桶的 counter，histogram/gauge 只留 type/service/method 控 series×桶 爆炸；③ratelimit key 注册期预存（`rule{lim,key}`），热路径零字符串分配（实测 47.6ns/1alloc→10.7ns/0alloc）；④metrics 注册到 `DefaultRegisterer` 复用现有 `/metrics`、不另起端点；⑤tracing 与已接入的 otelgrpc StatsHandler 功能重叠，二选一（本拦截器暂未接线）；⑥看板延迟热力图 + P50/P90/P99 参考 go-zero 官方模板设计，指标名用自有 `webook_grpc_requests_*`（未抄 go-zero 的 `rpc_server_*`）。
**待办**：①logging panic 是否升 Error 级便于告警；②`go-kratos/aegis` 与项目自带 `pkg/circuitbreaker` 二选一标准化；③看板 heatmap 与其余面板的 rate-interval 统一；④tracing 是否接线（与 otelgrpc 取舍）。
**会话**：260625-grpcx-拦截器全家
**发布**：（拦截器库 + 看板配置，随后续服务发版生效）

## [2026-06-21] gRPC 自定义负载均衡器套件（SWRR / 熔断 / VIP 分组）

**变更内容**：`pkg/grpcx/balancer/swrr` 铺平三个平滑加权轮询（SWRR）balancer——`custom_swrr`（按权重平滑分流）、`breaker_swrr`（应用级熔断：连续失败摘除 + 冷却半开探活恢复）、`group_swrr`（按请求 `x-tier` metadata 分流到节点组，如 VIP 池），三者共享 `conn` + `swrrPick`；配套中立标签包 `balancer/{weight,group}`（resolver 写、balancer 读，互不依赖）；`resolver/etcd` 改编官方 etcd naming/resolver，watch 时额外下发带权 + 带组 `Addresses`。**已接入 chat→core 调用链**：`client.go` 换带权 resolver + 按 `ClientConfig.Balancer` 选均衡器、`server.go` 注册端点带 weight metadata；chat 默认 `breaker_swrr`、core 注册 weight。
**影响范围**：新增 `pkg/grpcx/balancer/{swrr,weight,group}`、`pkg/grpcx/resolver/etcd`；改 `pkg/grpcx/{client,server}.go`（+Balancer/+Weight）+ `{internal,chat}/config/*.yaml` ×10 接入；测试覆盖三 balancer（真链路 + 白盒熔断）+ 标签包 + metadata 解析。
**技术决策**：①`base.NewBalancerBuilder` 只读 `State.Addresses`、不读 `Endpoints`，故 weight/group 经 `Address.Attributes` 携带（`BalancerAttributes` 已 deprecated 且参与 `Address.Equal`，弃用）；②官方 etcd resolver 只填 `Endpoints` 且丢 Metadata，base 系拿不到地址/权重，故 fork 后补填带标签 Addresses；③熔断状态挂 Picker 层（gRPC 重建会重置，轻量取舍）；④VIP 分组用请求 metadata 路由（无 xDS 控制面时的标准做法）。
**会话**：260621-grpcx-负载均衡器套件
**发布**：（基础设施代码，随后续服务发版生效）

## [2026-06-17] gRPC + etcd 服务注册/发现集成

**变更内容**：`pkg/grpcx` 收敛 gRPC「注册自己 / 发现下游」——`NewServer`（租约 + 自动续租 + 中断重注册 + 优雅注销）/ `NewClient`（etcd resolver + 按 `ClientConfig{Target,Secure,CAFile}` 构造凭证），otel/拦截器经 `opts` 由调用方传；抽 `pkg/viperx`（`LoadLocal`/`WatchRemote`）统一三服务配置引导，`pkg/netx` 探出口 IP。core 注册自己、chat 经 resolver 发现 core，main 接 SIGTERM 优雅停机 + gRPC 健康检查；配套 `sandbox/grpc/registry` 可运行 demo + `prd/grpc-etcd` 架构文档。
**影响范围**：新增 `pkg/grpcx`（server/client/interceptor）、`pkg/viperx`、`pkg/netx`、`{internal,chat}/ioc/etcd.go`；改 `{internal,chat,migrator}/main.go`、`internal/ioc/grpc.go`、`chat/ioc/grpc.go`、15 份 config yaml（`etcd.endpoints` 列表 + grpc 段）、wire；`go.mod` 锁 `etcd client/v3 v3.6.12`。
**技术决策**：①etcd 既做配置中心又做注册中心，复用同一集群；②横切 option 调用方传、grpcx 不内置默认；③`etcd.endpoints` 未配置即 fail-fast（注册/发现侧），不静默兜底 localhost；④服务名统一 `webook-core` + `service/` 前缀，client target 约定 `etcd:///service/<name>` 零配置接入。
**会话**：260617-grpc-etcd服务注册发现
**发布**：（基础设施代码，随后续服务发版生效）

## [2026-06-14] 监控告警修复与完善（NaN 误报 + 邮件链路）

**变更内容**：①prometheus 录制规则加 NaN 守卫（`分母 >0` / `and 观测计数 >0`），消除无流量时 5xx/分位/比率 `0/0=NaN` 抖出的 fire+instant-resolve 误告警；②grafana 邮件链路：root_url 改 `GRAFANA_ROOT_URL` 按 env 注入（修链接端口/协议）、contactpoints 模板 resolved 不显示 Summary、新增默认 `isPaused` 的冒烟测试规则验证「规则→通知→SMTP」整链。
**影响范围**：`deploy/prometheus/rules/*.rules.yml`（11 条加守卫）、`deploy/grafana/provisioning/alerting/*`（contactpoints + 新增 webook-smoke-test.yml）、`docker-compose.yaml`、8 份 `.env.*`（加 GRAFANA_ROOT_URL）。纯配置，prometheus `/-/reload` + 重建 grafana 生效，不动服务镜像。
**技术决策**：NaN 是「有数据」绕过 `noDataState`，守卫让无流量时序列消失 → 交 noDataState:OK；root_url 用完整 URL 变量以支持 https / 自定义端口。
**待办**：dev/staging/prod 真实 `.env` 的 `GRAFANA_ROOT_URL` 经域名/IP 访问时改真实 host；QQ SMTP 授权码在私有仓 .env 明文，建议择机重置 + 重新 gitignore。
**会话**：260614-postman鉴权与监控告警
**发布**：（配置，reload 生效）

## [2026-06-14] webook-core 启用 migrator SDK 双写/切流（webook-core-v1.3.0）

**变更内容**：`migrator.sdk.enabled` 四环境 `false→true`，core 文章读写挂 RedisSwitchReader/DualWriter。默认 stage（无 Redis stage key）行为等价旧逻辑（仍读写 `published_article`），仅热路径 +1 Redis 查询、Redis 故障降级 SideOld。
**影响范围**：`internal/config/{dev,staging,prod,test}.yaml`、`deploy/.env.prod.example`（CORE_IMAGE_TAG→1.3.0）。tag `webook-core-v1.3.0` → CI 出 `ghcr.../webook-core:1.3.0`。
**技术决策**：启用 ≠ 迁移，只挂插件；实际切流需另设 `migrator:stage:published_article_v1` 键 + prod 建并回填 `published_article_v1` 表。CI 跳过 integration，SDK 路径靠 prod runtime + 本地集成测试验证。
**待办**：部署同步真实 `.env.prod` `CORE_IMAGE_TAG=1.3.0` + `./deploy.sh prod`。
**会话**：260614-postman鉴权与监控告警
**发布**：（webook-core-v1.3.0，待部署）

## [2026-06-14] webook-migrator postman/README 鉴权对齐 Bearer（webook-migrator-v1.2.1）

**变更内容**：postman 集合改 `Authorization: Bearer`（移除无效的 x-access-token 请求头）+ 新增 A0 授权登录文件夹（登录脚本自动回填 token）+ 变量统一命名；README A.5 curl 修 Bearer 头 / 端口 :8090 / `$token` 变量，B3 ES 注入 `--data-raw`→`-d`（兼容旧 curl）。
**影响范围**：`webook/migrator/scripts/postman.json`、`webook/migrator/README.md`。tag `webook-migrator-v1.2.1`（dev 资产 + 文档，镜像功能无变化）。
**技术决策**：migrator 与 core 共用 `jwtx.ExtractBearer`，只认 `Authorization` 头；x-access-token 仅是登录响应头，旧集合靠 `jwt.disabled` 蒙混、从未真鉴权。
**会话**：260614-postman鉴权与监控告警
**发布**：（webook-migrator-v1.2.1）

## [2026-06-11] webook-migrator 分层合规重构 + 命名全链统一 + 业务监控 + 文档对齐

**变更内容**：在干净 v1 基础上做架构质量收口——①**跨层引用清除**：service/web 层不再直引 `repository/dao`·`repository/cache`·`redis.Cmdable`，新增 7 个 repository（checkpoint/validate_log/dead_letter/audit_log/throttle/switch_state + 原 task）+ `service/replay`（ReplayDL 业务从 handler 下沉）+ cache 层 `SwitchStateCache`；②**命名全链统一**：DAO 实体去 `Migration` 前缀（`MigrationTask`→`Task`…，含表名 `migration_*`→`*` 与索引名）、`Id` 风格贯通（`TaskID`/`taskID`→`TaskId`/`taskId`，字段/变量/方法一体）、转换 helper 升维泛型 `pkg/slicex.Map`（删手写复数方法）、删死哨兵 `ErrStateConflict` + validate_log 索引自愈兼容逻辑 + GTID 占位澄清；③**业务监控补齐**：`webook_migration_lag_ms{task_id,side}` + `webook_migration_dead_letter_unreplayed{task_id}`（scrape 实采 Collector）+ grafana 面板恢复真实查询（删此前永远 No-data 的幽灵面板）；④**transform 文件结构对齐** source/sink（接口/registry/identity/mongo 分文件）；⑤三服务 `ioc/config.go` 同构统一。

**影响范围**：`webook/migrator/**`（16 包）+ 新增 `webook/pkg/slicex`；`webook/internal/ioc/config.go`（core 抽出）+ `chat/ioc/config.go` + `internal/migratorsdk/sdk.go` 注释；部署 `deploy/grafana/.../webook-migrator.json`；prd/migrator 全量同步（02-architecture·03-walkthrough·README·postman 等）；`webook/CLAUDE.md` 新增 4 条规则（层间模型转换 / 标识符 Id 风格 / 包内多实现文件组织 / 多服务布局）。全模块 `go build/vet/test ./...` 全绿、goimports 干净；`Migration*`/`migration_*`/`TaskID`/`toDomainList` 全仓 grep 零残留。

**技术决策**：分层修复严守「service 只依赖 repository 接口、横切中间件（audit）可直达仓储」；`Id` 风格选「向仓内既有约定收敛」而非 Go-initialisms（存量几百处 vs 2 离群点）；`slicex.Map` 自写 6 行泛型而非引 samber/lo（标准库无 Map、避免新依赖）；表名前缀剥除因控制库是专用库 `webook_migrator`（前缀无命名空间价值）+ v1 未发版零迁移成本；监控用 scrape-time Collector（无后台 goroutine，2s 超时兜底 DB 聚合查询）。

**待办**：CountUnreplayedByTask 补 sqlmock 单测；full 同步进度指标（v2）；lag/dead_letter 告警规则（alerting yml，可选）；部署侧 dev/staging 旧 `webook_migrator` 库需 DROP 重建（表已改名，AutoMigrate 不迁移）。

**会话**：260606-migrator-删反向同步（延续）

## [2026-06-08] webook-migrator 清扫为干净 v1（删 CLAUDE.md/02b + 清除 idempotency/RBAC/反向同步残留 + 文档同步）

**变更内容**：把 migrator 代码注释与设计文档统一收敛为「干净 v1」——①删 `webook/migrator/CLAUDE.md`（v1/v2/扩展点前瞻框架最集中、且与 prd 文档重复）+ `prd/migrator/02b-task-module-design.md`（交付前 vertical-slice 蓝图，已被实际代码取代）；②清除文档里**已从代码删除的功能**残留：idempotency 中间件 / `Idempotency-Key` header / `migrator:idempotency` 键 / IdempotencyCache、RBAC 中间件假声明（`rbac.go` / `InitRBACBuilder` / 401·403·429 e2e / 挂 14 endpoint）——区分保留 `幂等`（Sink upsert 性质）；③代码注释去兼容性/前瞻措辞（反向同步残留 4 处、向后兼容、GTID/throttle v2 前瞻、Step 路标、失效声明 sink 乐观锁/canal 后续集成）；④文档前瞻指针（"以 CLAUDE.md 设计范围/v2 范围为单一真相源"）全部清掉，v1 能力以 `02-architecture.md`「📌 v1 实现摘要」为权威。

**影响范围**：删 2 文件；代码 12 文件改注释（build + vet + `go test ./migrator/...` 全绿）；prd 文档 `02-architecture.md`·`03-walkthrough.md`·`01-product.md`·`04-cutover-playbook.md`·`04b-cutover-checklist.md`·`adr/0002`·`assets/prometheus-alerting.yml` + `webook/migrator/README.md`·`scripts/postman.json` 同步；全仓 grep 验证 0 残留（idempotency 中间件 / Idempotency-Key / migrator:idempotency / IdempotencyMiddleware / RBAC middleware / requireRBAC / InitRBACBuilder / 反向同步 / webook/migrator/CLAUDE.md）。

**技术决策**：CLAUDE.md 与 prd/migrator 文档职责重叠 + 是 v1/v2 框架残留集中地，删除后 `02-architecture.md`（本就是「权威定义」）单一承载 v1 能力摘要；idempotency/RBAC 中间件早在 v1 收尾已从代码删除（见 `[2026-05-27]` 条），但文档当时未同步干净，本次彻底对齐；`幂等`（Sink ON DUPLICATE KEY + Version 乐观锁）是真实 v1 行为，与「idempotency 中间件」区分保留；反向 repair `dst_overwrite_src`（≠反向同步引擎）保留。

**待办**：无（v1 文档/代码已自洽；`webook/migrator/CLAUDE.md` 已删，先前条目对其「v2 设计范围」的引用随文件移除而失效，v1 文档不再前瞻 v2）。

**会话**：260606-migrator-删反向同步（续接）

**发布**：（未发布）

## [2026-06-06] webook-migrator prd 文档全面对齐 v1（反向同步归 v2 + 业务 metric 归基础设施）

**变更内容**：prd/migrator 设计文档与 v1 代码对齐——①移除反向同步（cutover Plan B），回滚模型改为「DST_ONLY 不可逆，回滚仅双写期 SRC_FIRST/DST_FIRST，切前充分对账兜底」；②42 处 `webook_migrator_*` 业务 metric 改为 v1 实际可观测（基础设施 metric `webook_http_/db_/go_` + 控制台 API `/lag`·`/tasks/:id`·`/mismatch` + mysql 查控制库 + 服务日志），prometheus-alerting 重写为 v1 4 条核心告警 + 业务 SLO；③health `{status:ok,service}`、throttle「next_start 生效」、Redis stage/gray key 用 taskName —— 全部对齐代码。
**影响范围**：`prd/migrator/`（01~04b + 10 runbook + adr/retros/assets）+ `webook/migrator/CLAUDE.md`（v2 设计范围补反向同步）；代码侧反向同步增删抵消（`pipeline/source/factory.go` 等回原样，build/vet/44 单元包全绿）。
**技术决策**：反向同步（NEW→OLD 自动回写）涉及引擎方向化 + 防死循环 + 状态编排，复杂度高，v1 不做、归 v2，靠「切单写不可逆 + cutover 前强制对账（mismatch<0.001%）」兜底；业务 metric 埋点归 v2（落地须合规命名 `webook_<subsystem>_*`），v1 用基础设施 metric + API 可观测。
**待办**：v2 若做反向同步 / 业务 metric，按 `webook/migrator/CLAUDE.md`「v2 设计范围」推进。
**会话**：260606-migrator-删反向同步
**发布**：（未发布）

## [2026-05-28] webook-migrator v1 收尾（异构 verify Mongo dst + 共享 mongo client + README A/B/C + 全量文档同步）

**变更内容**：在 v1 任意源框架（2026-05-27）基础上补齐两件事：①**异构 verify 真支持 Mongo 作目标**（`SourceFactory.BuildDst` mongo 分支复用 `MongoSource` 读 dst collection + `VerifyEngine.diffAndLog` 比对前 `normalizeRows` 对两侧应用表的 transform 归一 + 去 `_id` PK 回显）；②**README 重排为 A/B/C 三组**（A 核心生命周期 / B 异构方向 / C 参考），修 Step 4 空号 + 6b 编号、ES Appendix B 上移成 B3、顶部加目录、内部交叉引用全部对齐。

**影响范围**：
- 代码（前一会话已实现，本日补 e2e + 文档）：`pipeline/source/factory.go`（BuildDst mongo 分支）、`service/verify/verify.go`（`normalizeRows` + transform Registry 注入）、`ioc/engines.go::InitVerifyEngine`、`service/verify/verify_test.go`（异构-transform 用例）。
- 新增 e2e：`integration/verify_mongo_e2e_test.go::TestMySQL_E2E_VerifyMongoDst` —— 真 infra 跑 MySQL→Mongo 全量后构造 VerifyEngine（mysql+mongo 双 builder + transform Registry）跑 Full，断言①干净迁移零假阳性 ②改 dst 一行检出恰好 1 diff。**本机 Mongo 副本集 + MySQL 起着时实跑 PASS**。
- 文档：`webook/migrator/README.md` 整篇重排（1290→1319 行，命令/期望输出零流失；fence count 140=140 校验）；`webook/migrator/CLAUDE.md`「已知功能边界」加「异构 verify Mongo dst ✅」行；`verify.go` 包注释边界更新。
- 顺手修 bug：README L1155 `mongosh` 命令漏 `&authSource=admin` → 用户运行报 `Authentication failed`。补齐后与其他 5 处 mongosh 命令一致。

**技术决策**：
- 异构 verify normalize 选「两侧都过 transform + 去 `_id`」而非「只对源端 transform」—— transform 对已正确迁移的 dst 是幂等的（拍平后再拍平不变），保证同构（Identity + 无 `_id`）路径行为零漂移，单测验证 2 路径（异构 + 同构）。
- README 重排走 Python line-range 切片 + 标题行重写脚本（命令体 byte-for-byte 切片，不是 LLM 重写命令），避免命令文本回写漂移；脚本输出与原文做 `Counter` 比对验证零内容流失。
- 编号方案选 A/B/C 三组但不动 markdown heading 层级（组首和子节都是 `##`），避免大规模层级 demotion 风险；目录用手写嵌套列表，导航靠 Ctrl-F + 目录文字定位（不依赖 anchor 链接，避免中文/标点 anchor 生成歧义）。

**配套收尾**：①`ioc.InitMongoClient` 进程级共享 `*mongo.Client` 注入 wire（sink/source 各 builder 复用同一连接池，driver 内置 pool 管理），关闭原 `buildMongo*` 多 client 模式；②全部迁移相关文档（`migrator/CLAUDE.md` / `migrator/README.md` / `prd/migrator/01-product.md` / `02-architecture.md` / `02b-task-module-design.md` / `03-walkthrough.md` / `04b-cutover-checklist.md`）同步对齐 v1，删除"首版差异 / 待办 / TODO"语气，统一指向 `migrator/CLAUDE.md`「设计范围与扩展点」作为单一真相源。

**会话**：260527-migrator-任意源框架（续接 2，从 2026-05-27 晚 prompt-too-long 中断 → 2026-05-28 收尾）
**发布**：待发布

---

## [2026-05-27] webook-migrator v1（第一版）

**总述**：webook-migrator 第一版 —— 异构数据迁移服务（任意源 MySQL/Mongo ↔ 任意目标 MySQL/ES/…），全量 + 增量（CDC binlog / Mongo Change Stream）+ 对账 + 4 阶段切流。本条目合并三段建设：①完整 review（17 bug + 异构对账闭环 + canal cdc 落地 + 架构简化 + 文档同步，§一~五）；②incr binlogpos→checkpoint 持久化修复（§六）；③任意源框架（MySQL 单源 → 任意源可插拔，Mongo 全量+增量，§七）。**Mongo 全量 + 增量 e2e 已对真副本集 Mongo（2 节点 rs0）验证 PASS**。

**背景**：以 README Step 8 对账数据对不上为入口，一路 review 出来一连串关联 bug + 设计缺陷。最终覆盖：P0 阻塞修复、P1 实现缺失补齐、异构对账闭环、canal cdc 运行环境落地、架构 YAGNI 简化、prd 文档同步。完整 17 个 bug 表见 `webook/migrator/CLAUDE.md`「已知功能边界」段。

### 一、P0 + 关键 bug 修复

| Bug | 修法 |
|------|------|
| SourceFactory cdc 任务全量阶段误用 CanalSource | 接口拆 `BuildFullSrc / BuildIncrSrc / BuildDst` 三方法（按读取语义,不再按 task.Mode）;ADR-0002 记录决策 |
| validate_log 唯一索引 `uk_dedup` 含 repaired 列 → mark 时 1062 Duplicate entry | 索引列减为 `(task_id, table_name, biz_id)`;`BatchInsert` upsert 显式重置 `repaired=0`（绕开 GORM `default:0` 零值过滤） |
| `task.status` 死字段 8 状态全停 0 | `TaskService/Repository.UpdateStatus`;FullEngine/IncrEngine.Run 入口/出口 defer 推进;SwitchService DST_ONLY 同步 Switched |
| DSN 字段被忽略（所有迁移在控制库内自闭环） | 新建 `pipeline/dsn/Resolver` 接口 + `StaticResolver` 占位;SourceFactory/SinkFactory 加 `WithDBResolver`;生产前补 Vault PerTaskResolver |
| audit_log.error_msg 永远空 | middleware 解响应 body `{code,msg}` 截 512 字节填入 |
| Preflight 占位返 ready=true | 通过 `dsn.Resolver` 拿源端 db,真查 `@@global.binlog_format / gtid_mode` + `information_schema.STATISTICS` 表 PK |
| `/lag` 只看 src 侧 | 加 `IncrEngine.LagDst` + `dstLastEventTs sync.Map` CAS 更新;handler 返 `{lagMs, srcLagMs, dstLagMs}` |
| GORM AutoMigrate 不改同名索引 | `dao/init_table.go` 加 `ensureValidateLogDedupIndex`:启动时查 information_schema 比对,不符合就去重 + DROP + ADD 自愈 |
| GORM 零值字段过滤让 upsert repaired=0 失效 | DoUpdates 用 `clause.Assignments(map{"repaired": 0})` 字面值绕开 |
| `/start` 重复请求双开 race | `paused.LoadOrStore(taskID, nil)` 原子占位 + handler `IsRunning` 早判 409 双层防护 |
| `/verify sampleRate=0` 不被拦 | SampleRate 改 `*float64` 区分"未传"vs"显式 0";handler 校验 (0,1] |
| `/gray percent=200` 业务消息被吞 | 去掉 binding `min/max` tag,让 service 层 `ErrInvalidGrayPercent` 透传到 HTTP |
| switch `Stage required` 让 rollback `stage:""` 撞 binding 400 | switchReq.Stage 去 required,handler 内仅 rollback 模式跳过 stage.Valid() |
| 状态机错误 sentinel.Message 吞动态 from/to | `WithMetadata` 携带 `from/to/allowed`,前端 `Result.Metadata` 取 |
| README Step 8.1 INSERT id=99 重复跑撞 PK | 改 `REPLACE INTO`,Step 8.1 全部幂等 |
| Step 9 `mark_only` 后再 verify 同差异从列表消失 | DoUpdates 覆盖 repaired=0 让差异重新进列表（符合"当下承认,再发现仍提醒"语义） |
| Windows Git Bash `paste -sd,` \r\n 导致 ids JSON 无效 | README 改用 `tr -d '\r' \| tr '\n' ',' \| sed 's/,$//'` |

### 二、异构对账闭环（MySQL ↔ ES 双向真异构）

- 新建 `pipeline/source/es.go`（~190 行）：ESSource 用 search_after 分页（ES 8 scroll 已 deprecated）+ aggs PKRange + IncrSubscribe 返 err（ES 无 binlog 概念）
- `SourceFactory.BuildDst` 按 `task.SinkType` 分发 MySQL/ES;`WithESSourceBuilder` ioc option
- 与 `buildESSink` 共享 yaml `migrator.es.addrs`（读/写同集群）
- `es_test.go` 用 httptest.Server 作 mock（esapi.SearchRequest.ctx 是 unexported,不抽 ESClient interface,直接传 `*elasticsearch.Client`）;6 case 全 PASS
- README 附录 B 完整 demo：B.1 创建 → B.2 全量同步 → B.3 真对账 → B.4 src_overwrite_dst 真覆盖 ES → B.5 切流

### 三、canal cdc 运行环境落地

**协议层（之前已实现）**：`GoMySQLCanalClient`（go-mysql canal SDK）+ `CanalSource` + `BinlogClient` 接口。

**本轮补齐**：
- `config/local.yaml` + `test.yaml` 加 `migrator.canal.{addr,user,password,serverIdBase,flavor}`
- `deploy/docker-compose.yaml` MySQL `command:` 加 `--log-bin --binlog-format=ROW --binlog-row-image=FULL --server-id=1 --default-authentication-plugin=mysql_native_password`
- 新建 `deploy/mysql/init/01-canal-user.sql`：首次启动建 canal 用户 + REPLICATION SLAVE/CLIENT/SELECT 权限
- 新建 `integration/canal_e2e_test.go`（~150 行）：连真 MySQL,INSERT/UPDATE/DELETE 触发 BinlogEvent 到达;基础设施不可用 t.Skip
- README Step 6b cdc 完整 demo + 故障排查表

**canal 自动重连**（architect review P0 修复）：
- `Subscribe` 包重连循环,指数退避 1s→30s 封顶;`canalEventHandler` 跨重连周期共享 + `OnRotate/OnPosSynced` 实时跟踪最新位点
- `canalSrv` 改 `atomic.Pointer[canal.Canal]`,重连换新实例时 Stop 安全

### 四、架构简化（YAGNI / 删过度抽象）

- 删 `web/middleware/{rbac,idempotency}.go` + `repository/cache/idempotency.go` + `integration/authz_e2e_test.go`：RBAC 是占位 NoOp,Idempotency 调试期反而干扰;不可逆操作（switch/repair）由 MySQL 唯一索引 + 状态机 + IsRunning 兜底
- `ChangeEvent` 改成 `type ChangeEvent = BinlogEvent` 别名（字段完全一致,无业务转换）
- `BinlogClient.Subscribe(ctx, fromPos, fromGTID)` → `Subscribe(ctx, fromPos)`（fromGTID 一直 `_` 忽略,接口诚实优于占位）;CursorKindGTID 改返显式 error
- 删 es_test.go / canal_e2e_test.go 末尾的 `var _ = errors.New` 占位

### 五、prd/migrator 文档同步

- `02-architecture.md` 头部加"📌 与首版实现的差异"段（12 行表格映射目标态 vs 首版实际）
- `02b-task-module-design.md` / `03-walkthrough.md` 顶部加 caveat 指向差异段;`03 §8.2` SourceFactory 代码示例改新三方法接口
- `01-product.md` / `04b-cutover-checklist.md` 加 RBAC/Idempotency 实施差异注释
- `04-cutover-playbook.md` + 7 个 runbooks：批量 sed 删 `-H "Idempotency-Key: $(uuidgen)"` curl header 行;删 `IDEM()` shell helper + 13 处 `-H "$(IDEM)"` 调用
- 新增 `prd/migrator/adr/0002-source-factory-three-methods.md`：详细记录 SourceFactory 三方法决策的备选/理由/后果

### 六、incr binlogpos → checkpoint 持久化修复（合并 2026-05-26）

Canal 端 `buildBinlogEvent` 漏填 `BinlogPos` + `OnRow` 没透传 file/logPos → `BinlogEvent.BinlogPos==""` → `runPartition.flush()` 守卫 `if lastPos!="" && compareBinlogPos>0` 永不命中 → `migration_checkpoint` `phase=incr` 行从未入库 → 重启后从 master 当前位点起订，丢重启窗口事件。修复：`OnRow` 透传位点 + `buildBinlogEvent` 加 binlogPos 参数（`canal_client.go` + `canal_client_test.go` 3 子测试防回归）；首事件 file 空时留空串不写残废格式（守卫逻辑本身正确，从源头修）。

### 七、任意源框架：MySQL 单源 → 任意源可插拔（Mongo 全量+增量，合并 2026-05-27）

4 阶段：①PK 行标识全链路 int64→string（`Row`/`ChangeEvent`/`Mutation.PK` + `validate_log`/`dead_letter.biz_id`→`varchar(64)`），数值源分片键/全量游标保数值语义（**拆两层规避 `"10"<"9"`**）；②可插拔骨架：`domain.SourceType`(mysql/mongo) + `SourceFactory` 按源分发 `BuildFullSrc`/`BuildIncrSrc` + 新 `pipeline/transform`（`Transformer` 接口 + `Registry` 按 `TableMapping.Transform` 名选，空→Identity）；③`MongoSource` 全量（find 单 shard 流式 + ObjectID→hex）+ `MongoToRelational`（嵌套子文档/数组→JSON 列）+ full 引擎接入；④`MongoSource` 增量（Change Stream `Watch` + resume token 经 `BinlogPos` 复用引擎游标）+ incr 引擎接入。关键决策：**全量游标 int64→string「最后发出的 PK」**（源升序 last==max，零 MySQL 漂移 + 通吃非数值 PK）；transform 注册表建在 ioc `InitFullEngine/InitIncrEngine` 内（引擎 provider 签名不变，免 wire 重生成）。`go vet ./migrator/...` 全净（顺手清掉 `runPartition` pre-existing unreachable code）。新增 `pipeline/source/mongo.go` · `pipeline/transform/` · `consts.CursorKindResumeToken` · `deploy` 加 `webook-mongo` 单节点副本集 · `config` mongo 段 · `integration/mongo_{,incr_}e2e_test.go`。**两 e2e 对真副本集 Mongo 验证 PASS**。

### 八、扩展点 / v2 路线图（按生产场景演进）

v1 已交付全集功能；下列项为接口扩展点（v1 已抽象，按生产场景注入实现）或显式 v2 范围（需架构决策）。完整对照见 [`webook/migrator/CLAUDE.md`](webook/migrator/CLAUDE.md)「设计范围与扩展点」。

1. **DSN 真 Vault/K8s Secret 解析**：实现 `pipeline/dsn.PerTaskResolver` 替换 `StaticResolver`，按 `task.SourceDsnRef`/`SinkDsnRef` 解明文 DSN + LRU `*gorm.DB` 连接池；同模式扩展异构 sink（按 `task.SinkDsnRef` 支持多 ES/Kafka/CK/Mongo 集群）。
2. **多 cdc task 共享 binlog stream**（架构演进，per-task → per-process）：单进程一个 canal client + `IncludeTableRegex` 全订；`IncrEngine` 按 `task.tables` 过滤事件。影响 `BinlogClient` 接口，独立立项。
3. **Canal GTID 续订模式**：`BinlogClient.Subscribe` 加 `fromGTID` 参数 + `GoMySQLCanalClient` GTID 实现 + MySQL `--gtid-mode=ON --enforce-gtid-consistency=ON`。
4. **CK / Kafka verify-dst Source**：在 `SourceFactory.BuildDst` 加 switch case 复用对应 Source 实现（参考 mysql / es / mongo 分支模板，~200 行各）。
5. **RBAC scope 中间件回接**：webook-core 起 SSO 签发链路后，挂回 `web/middleware/rbac.go` 走 `migrator:{read,write,switch,repair}` scope 校验。
6. **Throttle 运行时实时调速**：`engine` 暴露 `SetConfig` + atomic 字段实时反映 yaml/Redis 改动。
7. **Canal Prometheus 指标增强**：`connection_status` / `events_total` / lag 平稳态（无事件时返 0 而非 stale）。
8. **dev/staging/prod yaml 按部署目标补完**：local.yaml 已配 `migrator.{canal,mongo,es}.*` 全集；其他环境按部署目标库参数复制即可。
9. **Mongo `_id` 全量游标 mid-shard resume**：`Source.FullScan` 签名扩展（影响所有 source）；v1 走 `MongoSink.ReplaceOne` upsert 幂等重扫兜底，已知设计选择。
10. **2026-05-26 之前的 webook_migrator 库一次性升级**：旧实例如已跑过 cdc，`migration_checkpoint.phase='incr'` 行可能为空（§六 修复前的现象），重启前手工 INSERT 一行兜底位点；见下「部署侧手动迁移 SQL」段。

### 部署侧手动迁移 SQL（已有 webook_migrator 库升级前必跑）

```sql
-- 1. validate_log 去重 + 换索引（去 repaired 字段）
DELETE v1 FROM migration_validate_log v1
  INNER JOIN migration_validate_log v2
  ON v1.task_id=v2.task_id AND v1.table_name=v2.table_name
     AND v1.biz_id=v2.biz_id AND v1.id < v2.id;
ALTER TABLE migration_validate_log
  DROP INDEX uk_migration_validate_log_dedup,
  ADD UNIQUE INDEX uk_migration_validate_log_dedup (task_id, table_name, biz_id);

-- 2. webook-mysql 容器升级让 docker-compose 新参数生效。**推荐方式（保数据）**:
-- 手动跑一次 canal 用户 SQL,然后 restart 容器,不删 volume:
-- mysql -h 127.0.0.1 -uroot -p13520 < deploy/mysql/init/01-canal-user.sql
-- docker compose -p webook-local restart webook-mysql
--
-- ⚠️ 暴力方式(会清 webook / chat / migrator 所有库,**先 dump 备份**):
-- docker exec webook-mysql mysqldump -uroot -p13520 --all-databases > /tmp/backup.sql
-- docker compose -p webook-local stop webook-mysql
-- docker volume rm webook-local_mysql-data
-- ./deploy.sh local

-- 3. 清残留 Idempotency Redis key（可选）
-- redis-cli -a <pass> KEYS 'webook:idem:*' | xargs redis-cli -a <pass> DEL

-- 4. 任意源框架 schema 升级（已有 webook_migrator 库,来自 05-27）：
ALTER TABLE migration_task ADD COLUMN source_type varchar(32) NOT NULL DEFAULT 'mysql' AFTER kind;
ALTER TABLE migration_validate_log MODIFY COLUMN biz_id varchar(64) NOT NULL;
ALTER TABLE dead_letter MODIFY COLUMN biz_id varchar(64) NOT NULL;
```

**会话**: 260525-migrator-Step8-对账调试 + 260526-migrator-binlogpos修复 + 260527-migrator-任意源框架（v1 合并）
**发布**: 待发布

---

## [2026-05-19] webook-migrator 部署 / CI / 监控同步（补齐 CLAUDE.md 服务拆分 14 项）

**变更内容**: 补齐 2026-05-16 migrator 服务初版遗留的 9 项部署/CI/监控同步，使其与 webook-core / webook-chat 处于同一就绪等级，可走 `./deploy.sh <env>` 起服务、被 Prometheus 抓取、Grafana 看板/告警生效、GitHub Actions 自动构建镜像

**影响范围**:
- 新增 `webook/migrator/Dockerfile`（多阶段构建，抄 chat/Dockerfile 改 build target = `./migrator`）
- 新增 `.github/workflows/webook-migrator-ci.yml`（lint-test + build-push，tag 模式 `webook-migrator-v*.*.*`）
- 新增 `deploy/grafana/provisioning/alerting/webook-migrator.yml`（up / 5xx / P99 / goroutines 4 类告警，`{job="webook-migrator"}` 限定）
- 新增 `deploy/grafana/provisioning/dashboards/webook-migrator.json`（8 panel：up + QPS + P50/95/99 + Go runtime 3 + 业务 lag/dead-letter 2）
- 改 `deploy/docker-compose.yaml`：加 `webook-migrator` service 段（depends_on mysql/redis、healthcheck :8083/health、mem_limit MIGRATOR_MEM 默认 384m）
- 改 `deploy/nginx/conf.d/default.conf`：加 `upstream webook_migrator` + `/api/migrator/` location（白名单 IP，剥 /api 前缀转 :8083）
- 改 `deploy/prometheus/prometheus.yml`：加 `job_name: webook-migrator` target `webook-migrator:8083/metrics`
- 改 `deploy/.env.{local,dev,staging,prod}` + 4 份 `.example`：加 `MIGRATOR_IMAGE_TAG` / `MIGRATOR_APP_ENV`（实际部署用最小集 2 个；example 完整含 `MIGRATOR_MEM/GOMEMLIMIT/GOGC` 5 个）
- 收尾延续：改 `deploy/docker-compose.local.yaml` 补 `webook-migrator` local override（chat/core/fe 都有，独缺 migrator → `./deploy.sh local` 会去拉 ghcr 失败）；`.env.local{,.example}` 加 `MIGRATOR_HOST_PORT=8083` 让 local override 暴露宿主端口
- 收尾延续：`webook/migrator/README.md` 加 0.7 docker compose 容器模式段（验证容器 + nginx + prometheus + grafana + CI 入口）+ 附录 A 业务侧 SDK 接入自测 8 步（搬自 `prd/migrator/code-reading-guide.md §16.5`）；§16.5 改成指针段引用 README 附录 A

**技术决策**:
- 不新建 `deploy/prometheus/rules/webook-migrator.rules.yml` — 现有 `webook-services.rules.yml` 已按 `sum by (job)` 自动覆盖任意新 job，migrator 加 prom job 即被记录
- dashboard 业务 metric 命名 `webook_migration_*`（subsystem 前缀），**不用** `webook_migrator_*`（service 前缀）— 遵 CLAUDE.md「禁止 `webook_<service>_*`」
- nginx `/api/migrator/` 默认仅放 docker bridge / 内网 IP（公网默认 deny），符合 architecture.md「迁移服务全闭网」要求
- 4 份 `.env.<env>`（实际部署）只加 IMAGE_TAG + APP_ENV，跟现有 CORE/CHAT 精简风格一致；4 份 `.env.<env>.example` 完整含 MEM/GOMEMLIMIT/GOGC，给部署者参考

**待办**:
- prod 上线前在 `.env.prod` 把 `MIGRATOR_IMAGE_TAG` 同步到推出的 `webook-migrator-v*.*.*` 真实 tag 号（同 CORE_/CHAT_ 规则）
- migrator 服务实装 `webook_migration_lag_seconds` / `webook_migration_dead_letter_total` 指标后 dashboard 业务 panel 自动生效

**会话**: 260519-migrator-部署同步

## [2026-05-16] webook-migrator 服务初版（多表 + Canal + 异构 Sink + 完整鉴权）

**变更内容**: 新增独立服务 `webook-migrator`（与 webook-core / webook-chat 并列），完整数据迁移框架 — 全量 / 增量 / 对账 / 切流 / 死信重放 14 endpoint + 业务侧 SDK 接入 + 三件套 PRD/架构/playbook 文档

**影响范围**:
- 新建 `webook/migrator/`（独立服务，与 chat 平级）：
  - `service/{full,incr,verify,switching}` 五大引擎；IncrEngine 含 partition 并行（FNV hash + errgroup + min-ckpt resume + ckpt 防回退）
  - `pipeline/source/`：`MySQLSource`（全量分页 SELECT）+ `CanalSource`（实现 `GoMySQLCanalClient` 基于 `go-mysql-org/go-mysql/canal` 真订阅 binlog）；`SourceFactory` 按 task.Mode 分发
  - `pipeline/sink/`：`MySQLSink`（INSERT ... ON DUP KEY UPDATE + Version 乐观锁）+ `ESSink` + `ClickHouseSink` + `MongoSink` + `KafkaSink`；`SinkFactory` 按 task.SinkType 分发
  - `domain.Task`：`Tables()` / `PickTable(idx)` 支持任务内多张表；`EncodeShardNo(tableIdx, shardNo)` 解决 checkpoint 表冲突
  - `web/`：14 endpoint（CRUD + Lifecycle 11 + Query）+ middleware（idempotency / audit / RBAC 4 scope）；handler 持 factory 运行时按 task.id 动态构造 Source/Sink
  - `repository/dao/`：5 张控制库表（task / checkpoint / validate_log / audit_log / dead_letter）
  - `config/`：5 份 yaml（local/test/dev/staging/prod），含 partitionCount + Canal / ES / CK / Mongo / Kafka 配置段
  - `ioc/`：8 Provider + InitRBACBuilder + Rate-Limit middleware
  - `integration/`：e2e（CRUD + idempotency + audit + 401/403/429 鉴权 5 子测）
- 新建 `webook/internal/migratorsdk/`：业务侧接入接口
  - `SwitchReader.ChooseSide` 按 stage + gray 决策 OLD/NEW 路由
  - `DualWriter.Write` 按 stage 分阶段双写策略
  - `NoOp` / `Redis` 两套实现 + `FailureRecorder` 双写失败兜底
- 主仓 `webook/internal/wire.go` + `migrator_sdk.go`：yaml flag `migrator.sdk.enabled` 决定注入 NoOp / Redis 实现
- 新建 `prd/migrator/` 文档套件：PRD / architecture / zero-downtime-playbook / task-module-design / code-reading-guide / cutover-checklist / 10 runbooks / drill-records / postmortems
- `webook/go.mod` 加 5 个异构 SDK 依赖：`go-mysql v1.15.0` / `elastic v8.19.6` / `clickhouse-go v2` / `mongo-driver v1.17.9` / `sarama`

**技术决策**:
- IncrEngine partition 并行：单一订阅 → dispatcher FNV-hash → N partition channel → 各 worker 攒批 / Sink.Apply / checkpoint。subscriber / dispatcher / workers 全进 errgroup，任一失败 gctx cancel 全部退出（无 goroutine leak）
- 多 partition resume 正确性：load 全部 partition ckpt → `min(各 partition CursorValue)` 作 IncrSubscribe 起点 + worker 保留 startPos 防 ckpt 回退（fast partition 重放安全靠 Sink 幂等 + Version 乐观锁兜底）
- BinlogPos 比较：先比 file 字典序（zero-padded `mysql-bin.000001` 保证字典序 = 数字序）再比 pos 数字（不能字典序 — `"100" < "99"` 字典序但 100 > 99 数值序）
- 任务内多表分发：checkpoint shard_no 编码 `tableIdx * ShardStride + realShardNo`（ShardStride=10000，每张表最多 1 万 shard，最多 21474 张表）
- factory 模式：引擎 / handler 持 factory，运行时按 task.id Get → BuildSrc/BuildDst 构造对应 Source/Sink；wire 不再注入静态 Source/Sink 实例
- Sink 异构分发：MySQLSinkFactory 默认 MySQL；task.SinkType=es/clickhouse/mongo/kafka → 调 heteroBuilder（ioc 注入，按 yaml 配 ES client / CK conn / Mongo Connect / Kafka SyncProducer）
- ESSink 乐观锁：external version_type + Mutation.Version → ES 自动拒老 version 写入（409 conflict，业务层视为正常跳过）
- ClickHouseSink：insert 走 ReplacingMergeTree(version)，delete 走 `ALTER TABLE ... DELETE`（CK 异步删除）
- KafkaSink：key=PK → HashPartitioner 同 PK 落同 partition 保单行顺序
- RBAC：4 scope（read/write/switch/repair）+ ScopeExtractor 函数式注入（生产 Redis lookup，本地 NoOp 全 scope）；挂到 14 endpoint
- Rate-Limit：复用 `pkg/ginx/middleware/ratelimit` 滑动窗口（默认 1 秒 100 req / IP，yaml 可覆盖）

**待办**:
- 部署到 dev 后跑端到端验证（部署同步已于 2026-05-19 补齐：Dockerfile / docker-compose / nginx / prometheus job + dashboard / grafana alerting + dashboard / .env MIGRATOR_* / GitHub Actions CI 全到位）

**会话**: 260516-migrator-完整版



## [2026-05-12] k8s 部署目录上移到仓库根 + 去冗余前缀

**变更内容**: `webook/k8s/`（10 个 YAML）整体上移到仓库根 `kubernetes/`，与 `deploy/` 并列；文件名去掉冗余的 `k8s-` 前缀
**影响范围**:
- `webook/k8s/k8s-*.yaml`（10 个）→ `kubernetes/*.yaml`（git rename，历史保留）
- `webook/mk/k8s.mk`：4 处路径改为 `../kubernetes/<name>.yaml`；顶部用法注释明确"必须在 webook/ 下执行"
- `webook/mk/infra.mk`：10 处路径改为 `../kubernetes/<name>.yaml`
- `webook/CLAUDE.md` L1 部署层章节：`k8s/`（将来式）改为 `kubernetes/`（已建）

**技术决策**:
- 位置上移：webook/CLAUDE.md 早有规划「K8s 与 `deploy/` 并列于仓库根，不挤压 deploy/」，本次落地
- 目录用 `kubernetes/` 全称：与 `deploy/` 一致采用通用全称，避免与 mk 文件名 `k8s.mk` 重复
- 文件去 `k8s-` 前缀：目录名已表意，`kubernetes/mysql-deployment.yaml` 比 `kubernetes/k8s-mysql-deployment.yaml` 干净
- Makefile 走 `../kubernetes/` 相对路径：调用方式 `make -f mk/k8s.mk` 隐含 working directory = `webook/`，顶部注释明确化避免误用

**会话**: 260512-k8s-目录上移



## [2026-04-28] 代码审查修复：鉴权收敛 + 缓存规则归位 + 配置一致性

**变更内容**: 修复连续三轮 ship review 发现的安全/规则问题 — 1 Critical（SSE ResumeStream 越权窃听他人对话流）+ 4 Important（IsGenerating 缺鉴权、ranking.Archive 与 ai.Dashboard 在 prod 暴露、UpdateContent 写后不清缓存）+ 多处 Suggestion（viper key 与 env 名不匹配致 prod 守卫失效、`// =====` 注释分隔线违规、chat local.yaml otel.env 标错、内部 config 注释漏 staging 维度）

**影响范围**:
- `chat/web/chat.go`：`ResumeStream` 增 UserClaims 提取 + `ListMessages` 归属探测（越权 → 404 阻断 SSE 建立）；`IsGenerating` 路由改 `WrapReqClaims[conversationIdReq, jwtx.UserClaims]`，handler 内同样走 ListMessages 探测
- `chat/repository/chat_message.go`：`UpdateContent` 写 DB 后自清缓存（Cache-Aside 完整）；删除 `DelMsgCache` 接口方法和实现 — 缓存职责回归 repository
- `chat/service/chat.go`：删除 `flushToDB` / `finalizeReply` / `savePartialReply` 中 3 处 `DelMsgCache(...)` 兜底调用
- `internal/web/click_event.go`：`/ai/dashboard` 加 `os.Getenv("DEPLOY_ENV") != "prod"` 路由守卫（与 ranking.Archive 同模式）
- `internal/web/ranking.go`：`/article/ranking/archive` prod 守卫从 `viper.GetString("deployEnv")` 改 `os.Getenv("DEPLOY_ENV")`（修复 gate 永不触发的 bug）
- `internal/web/article.go` / `internal/service/article.go` / `pkg/errs/error_test.go` / `pkg/errs/mapping_test.go` / `pkg/ginx/wrapper_errs_test.go` / `pkg/ginx/wrapper_variants_test.go`：7 处 `// =====` 分隔线改 Makefile 风格 `// ── 区域名 ──`（对齐 CLAUDE.md 注释风格）
- `chat/config/local.yaml`：`otel.env` 由 `"dev"` 改 `"local"`（避免 local trace 与 dev 服务器混存难分）
- `internal/config/local.yaml` + `test.yaml`：环境说明注释补齐 staging.yaml 一档（原文档过时只列 local/dev/prod 三档）

**技术决策**:
- ResumeStream 鉴权用 `service.ListMessages(uid, convId, 0, 1)` 当探测器，不新增 service 方法：复用既有「越权/不存在 → ErrConversationNotFound (404)」路径，不增加 API 表面积
- prod 守卫直读 `os.Getenv("DEPLOY_ENV")` 不走 viper：viper.AutomaticEnv 对非嵌套 key 会 lookup `DEPLOYENV`（uppercase 后无下划线），与 `.env.*` 里的 `DEPLOY_ENV` 对不上 — CLAUDE.md 已记「AutomaticEnv 对嵌套 key 不生效实测验证」，本次确认对扁平 key 同样有此陷阱
- UpdateContent 自清缓存而非保留 `DelMsgCache` 兜底：service 三处调用容易在新加调用点漏写；接口收敛后责任落到 repository 层，CLAUDE.md「写操作后必须清对应缓存」从字面规则变成结构保障
- Dashboard / Archive 用 prod 路由守卫而非 admin role middleware：项目当前没有 admin 角色概念，路由守卫是最小可行方案；二者性质一致（运维/调试用，业务无依赖）

**会话**: 260428-review-修复



**变更内容**: 新增 `pkg/redislockx`（bsm/redislock 底座 + 自研 Watchdog 续约 + OnLost 钩子）+ `pkg/cronx`（任务级 Prometheus 指标 + Wrapper 通用模板）；prometheus 子包对齐 `redisx/gormx` 的 builder 链式风格；`internal/job/ranking.go` 缩到薄壳，service 层完全不感知锁；archive 任务首次纳入分布式锁保护；cron 加 graceful Stop hook

**影响范围**:
- 新增 `webook/pkg/redislockx/`：`Client`/`Lock` 接口；`redisClient` 用 `bsm/redislock` v0.9.4 提供 Obtain/Refresh/Release，错误统一映射到本包 `ErrLockNotHeld`；`redisLock` 在 bsm 之上自研 Watchdog（redisson 招牌特性 bsm 不带）+ `OnLost` / `OnRefresh` 钩子（续约失败 / 续约成功可观测，回调内 panic 由 `safeOnLost` / `safeOnRefresh` 包 recover 防止拖崩进程）；**watchdog 默认 ttl/3 自动开**（对齐 Redisson `lockWatchdogTimeout/3` 行为，调用方无须显式启用）；`Options`：`WithWatchdog`（覆盖默认 interval）/ `WithoutWatchdog`（显式关闭）/ `WithRetryInterval` / `WithOnLost` / `WithOnRefresh`，`applyOptions(opts, ttl)` 助手统一去重
- 新增 `webook/pkg/redislockx/prometheus/`：builder 风格装饰器，自动给每次 TryLock/Lock 注入默认 OnLost；指标 `webook_lock_acquire_total{result=success/busy/error}` + `held_seconds` + `wait_seconds`（Lock 阻塞实际等待）+ `watchdog_lost_total`（锁中途丢失，幻觉持锁告警）
- 新增 `webook/pkg/cronx/`：`Wrapper` 把"抢锁→跑业务→Unlock + 4 组指标 + panic recover"模板封死，业务 Job 复用；3 个 Option（`WithNow` / `WithLockKeyPrefix` / `WithLockTTL`）；watchdog 由 redislockx 默认接管，wrapper 不重复暴露；recover 加 `runtime/debug.Stack()` 进 panic 日志
- 新增 `webook/pkg/cronx/prometheus/`：builder 链式风格，4 组指标 — `webook_cron_runs_total{task,result=success/failed/skipped/panic}`、`duration_seconds{task}`、`in_flight{task}`、`last_success_timestamp{task}`
- `internal/job/ranking.go`：缩到 ~50 行，只剩 entries 表 + `wrapper.Wrap()` 调用；4 个任务命名加 `ranking_` 前缀（`ranking_hot_recompute` 等）避免未来撞名；archive 任务首次纳入锁保护
- `internal/service/ranking.go`：`RecomputeHot/Best/New` 删除顶部 TryLock 模板，service 不再 import logger 用于锁日志
- `internal/repository/ranking.go` + `cache/ranking.go`：删 `TryLock` 接口方法和实现
- `internal/consts/cache.go`：删 `ArticleRankingLockPattern` + `ArticleRankingLockTTL`
- `ioc/lock.go`（新增）：`InitLockClient` 包 prometheus 装饰器；`ioc/cron.go`：`InitCron` 改返 `(*cron.Cron, func())`，cleanup 调 `<-c.Stop().Done()` 等 in-flight 跑完，进 wire cleanup chain；新增 `InitCronMetrics` + `InitCronWrapper`
- `wire.go` + `wire_gen.go`：注入 `InitLockClient` + `InitCronMetrics` + `InitCronWrapper`
- `mk/mock.mk` + mocks：`pkg/redislockx` 加入 mockgen 矩阵
- 测试新写：`pkg/cronx` 6（success/lockBusy/lockError/businessError/panic-with-stack/unlockIndependentCtx）+ `pkg/redislockx` 13（含 `DefaultWatchdog` / `WithoutWatchdog` / `OnLost`）+ `pkg/redislockx/prometheus` 4（含 watchdog_lost / wait_seconds）；`internal/job/ranking_test.go` 砍到 1 个入口测试（业务无关行为已被 cronx 单测覆盖）

**技术决策**:
- 底座选 `bsm/redislock` v0.9.4 而非自实现 SETNX+Lua：bsm 提供 token 校验、RetryStrategy 等成熟件，自研只剩 Watchdog（redisson 招牌特性 bsm 不带）；将来想换 redsync 多节点也只改 `client.go` 一处，`Client`/`Lock` 接口不动
- prometheus 装饰器靠"prepend WithOnLost"自动注入到每次 TryLock/Lock，**`NewClient` 签名零变更** —— 比加 ClientOption 工厂级默认更轻
- watchdog 默认开（ttl/3）对齐 Redisson：调用方"任何地方都能直接用"，无须每次记得 `WithWatchdog`；短临界区不想要后台 goroutine 用 `WithoutWatchdog()` 关
- service 层完全不感知锁：锁是部署形态决定的，不该污染业务接口
- Wrapper 抽到 `pkg/cronx`：业务无关模板，未来其他 Job 直接复用，无须复制 100 行
- Watchdog 30s TTL + 10s 续：实例 crash 30s 让贤；archive 任务 2min 也能持续续约
- task 名加 `ranking_` 前缀 + 锁 key 前缀 `cronx:lock:`：rolling deploy 容忍一次双跑（业务幂等），换来未来 Grafana 标签清晰
- 跳过 yaml 配置化（cron.lockTTL / watchdog 默认值已合理，无差异化需求时不开口子）
- 跳过 graceful shutdown 单测（chan 异步难真实，人眼审 wire_gen cleanup 链已够）

**监控模板**（同步随本次提交）：
- `deploy/prometheus/rules/webook-jobs.rules.yml`：**8 条 Recording Rules**（cron 成功率 + P50/P95/P99 三档耗时 + last_success_age + lock 错误率 / wait P99 / held P99）；模板版 `examples/recording-rules-example.yml`（与 `alerts-example.yml` 对仗的 config-type 命名，业务域无关）带完整注释。**Prometheus 规则目录从 `alerting/` 重命名为 `rules/`**，同步改 `prometheus.yml` 的 `rule_files: rules/*.rules.yml` + `docker-compose.yaml` 的 volume 挂载，与项目"Grafana Alerting + Prometheus Recording rules"架构对齐。**Dashboard 'Cron Duration' panel 改为读三档 record（perf 优化）**：每次 refresh 不再现算 `histogram_quantile`，CPU 开销由 N×panels 降至常量
- `deploy/grafana/provisioning/alerting/webook-jobs.yml`：6 条告警（CronTaskStale / CronPanicSpike / WatchdogLost / LockAcquireErrorRate / LockWaitP99High / CronInFlightStuck），全 Q→Reduce→Threshold 评估链路；模板版 `examples/alerting/rules-recording-example.yml`
- `deploy/grafana/provisioning/dashboards/webook-jobs.json`：14 panel 专题面板（cron 4 + lock 4 + 概览 4 + 健康 2），含 `$task` 变量过滤；模板版 `examples/dashboards/webook-jobs-example.json` 每 panel 带 description
- `webook/tools/check_monitoring.sh`：YAML/JSON 语法 + 元素数量校验脚本（promtool 不可用环境的替代）

**待办**:
- 部署到 staging 后观察 `webook_cron_*` 与 `webook_lock_*` 指标，配 Grafana 面板和 "X 分钟没成功过" / "watchdog_lost_total > 0" 告警
- transient Refresh error（非 ErrNotObtained）目前 silently retry 无指标，未来若发现 Redis 抖动场景需告警可加 `refresh_error_total` counter 或扩展 OnLost 在连续 N 次失败时触发

**会话**: 260426-定时任务-分布式锁与数据采集
**发布**: -

## [2026-04-23] 榜单移动端适配 + 切 tab 体验 + 后端并发 perf

**变更内容**: 榜单搜索页 5 项移动端响应式 + URL 状态持久化 + 200ms 延迟清空策略 + 后端并发优化 + trend bug fix；三份 CLAUDE.md 补硬规则

**影响范围**:
- 前端 `views/search/RankingBoard.tsx`：Header 堆叠、Tabs flex-wrap、Item meta 两行、Pagination simple + 对齐 `article/list.tsx`；URL 参数驱动（`dim/rcat/rdate/rpage/rsize`）；Spin 冻结 + minHeight 锁高 + 200ms 延迟清空；抽 `RankingItem` + `React.memo` 防 countdown tick 触发全量 re-render
- 全局 `app/globals.css`：`@layer tw-utilities` 全局 `scrollbar-gutter: stable`；`AppLayout.tsx` `<main>` 改 `overflow-y-auto`
- 后端 `repository/ranking.go`：`fallbackFromDAO` 删覆盖 snapshot trend 的 2 行（bug fix，归档 JSON 里原本就有 `trend:"up/down/new"` 被静默覆盖成 same）；`Top()` 用 `errgroup` 并发 `GetDetails` + `GetPrevRanks`
- 后端 `service/ranking.go`：`RecomputeHot` 5 分区 fan-out `errgroup`
- `scripts/webook.sql`：article / published_article 扩到 25 条；prev1 hot 归档扩到 25 条，满足 pageSize=20 分页阈值
- 原型：`prd/chat/chat.pen` 新增 `08-榜单-移动端` frame（tVVgY）；`prd/ranking/ranking-search-page-mobile.png`；PRD.md §6.2 移动端章节
- 规范：根 `CLAUDE.md` 加「侦察优先」「Pencil 原型修改」「动手前先出方案」；`webook-fe/CLAUDE.md` 加「列表分页状态规则」「跨页面模式对齐」「Effect 依赖稳定性」「常量集中」「CSS 规则就近 vs 全局」

**技术决策**:
- 移动端 pen 从 `rSwsj` 复制而非从零搭；硬编码进度条 width 按新 viewport 比例重算（225/175/158/119/99/86）
- URL 参数 `r` 前缀避免和 `/search?page=` 搜索分页撞名
- 切 tab 用"延迟 200ms 清空 + Skeleton 兜底"：快响应直接 commit 无感切换，慢响应才露 Skeleton
- `scrollbar-gutter` 挂在 `<main>` 而非 `html/body`（AppLayout `overflow-hidden` 下 body 不滚）；`@layer tw-utilities` 压过 Tailwind utility
- Pagination 完全对齐 `article/list.tsx`：`showTotal + showSizeChanger + showQuickJumper + size='small' + pageSizeOptions=['10','20','50']`

**待办**:
- 默认值魔数（`'hot'` / `'tech'` / `1` / `20`）可抽 `DEFAULTS` 常量
- `searchParams.get('dim')` 建议加白名单校验
- 748 行的 `RankingBoard.tsx` 可进一步拆 Header/Tabs 子组件

**会话**: 260423-ranking-移动端-规则沉淀

**发布**: 待补

## [2026-04-22] 文章日榜 Top100

**变更内容**: 搜索页落地文章日榜（热度/最新/最佳/分区 4 维度 Top100），支持实时刷新 + 次日归档 + 历史回看

**影响范围**:
- 后端全栈新增：`dao/ranking.go` / `cache/ranking.go` / `repository/ranking.go` / `service/ranking.go` + `ranking_scorer.go` + `ranking_hook.go` / `web/ranking.go`
- cron 业务层新建 `internal/job/` 目录（`ioc/cron.go` 只做生命周期）
- `article` / `published_article` 表加 `category` 列 + 索引；新增 `article_ranking` 表 + 视图 `v_article_ranking`
- 前端 `views/search/RankingBoard.tsx` + `api/ranking.ts` + `types/ranking.ts`；搜索页无 query 时默认落地榜单
- 通用工具：`pkg/ginx/page_result.go`（`PageResult{List any, Total int64}`），同步 `article.go` 两处分页接口复用
- 规范：`CLAUDE.md` 补"Handler/Service/Repo/DAO 构造函数必须返回接口"+"表名单数"

**技术决策**:
- 公式：热度 `(click + 3·like + 5·collect) / (hours+2)^1.5`；最佳 Wilson 下界（`clicks ≥ 50`）；最新按 `publish_ts`（24h + `clicks ≥ 10`）
- 存储：Redis ZSet 实时（1min 重算 + 增量 ZINCRBY）+ MySQL 归档；cron 分布式锁用 Redis SETNX
- cron 库：`robfig/cron/v3`，多实例靠 SETNX 抢占实现分布式
- date 字段：`varchar(10) 'YYYY-MM-DD'` 作日分区键（行业标准，非 int64 毫秒戳）；业务时区 **Asia/Shanghai**，三端对齐（后端 `carbon.Now()` 走全局 TZ、前端 `toLocaleDateString('sv-SE', {timeZone:'Asia/Shanghai'})`、SQL `CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', '+08:00')`）
- 接口命名：泛型接口 `RankingService/Repository/Cache/DAO`，实体实现 `ArticleRankingService` 等（未来 `UserRanking...` 可共享同接口）
- 分页响应：`ginx.PageResult{List any, Total int64}` 对齐前端 `types.PageResult<T>`
- 点击追踪：`ClickEvent.source` 编码为 `"ranking:{dim}:{rank}"`，不改表结构

**待办**:
- cron 仍是测试节奏（10s/15s/5s/10min），上线前切回生产节奏（1min/5min/30s/每日 00:10）
- `rankingCandidates=2000` / `bestMinClicks=50` / `newMinClicks=10` 可配置化
- 归档页面路由 `/ranking/archive?date=...` 未单独实现，通过 Drawer 切换日期达成

**会话**: 260422-ranking-文章榜单

**发布**: 待补

## [2026-04-21] v1.0.0 基础设施增强：端口避让 + ghcr 源切换 + 懒拉镜像

**变更内容**: 围绕 prod 正式发版 (`webook-v1.0.0` / `webook-fe-v1.0.0`) 做了三件部署基建的事：

1. **prometheus 宿主端口重排**：dev/staging/prod 从 9090/9091/9092 → 9090/9190/9290，与 `KAFKA_EXTERNAL_PORT`（9094/9194/9294）百位 tier 对齐；prod 原 9092 撞 Kafka 业界默认端口
2. **ghcr 镜像源可切换**：`.env.*` 加 `GHCR_REGISTRY`（默认 `ghcr.io`），`deploy.sh` 加 `--ghcr <host>` / `--ghcr=<host>` flag 单次覆盖（不改 env 文件）；配合 README 新增的阿里云 ACR 反代教程，为国内 prod 拉取慢提供方案
3. **up 不再自动拉镜像**：compose 17 个 service 全加 `pull_policy: missing`，`deploy.sh` 的 `up` 分支删掉 `$COMPOSE pull`——镜像缺失才拉，已有直接启动；想显式刷新走 `./deploy.sh <env> pull`

**影响范围**:
- `deploy/docker-compose.yaml`（webook/webook-fe image 路径变量化、17 service 加 pull_policy）
- `deploy/deploy.sh`（新增 `--ghcr` flag 解析、删 up 里的 auto pull）
- `deploy/.env.{local,dev,staging,prod}` + 4 份 `.example`（新增 `GHCR_REGISTRY`、改 `PROMETHEUS_PORT`）
- `deploy/README.md`（端口表 + ghcr 切换节 + 阿里云 ACR 反代节 + pull_policy 说明）

**技术决策**:
- **端口**：百位 tier 对齐 kafka 系，dev 保持社区默认 9090；Grafana → Prometheus 走容器内 `webook-prometheus:9090`，不受宿主端口改动影响
- **ghcr 源**：只做 ghcr，不碰 Docker Hub（后者走 daemon `registry-mirrors` 更标准）；CLI flag 用 shell export 覆盖 `--env-file`，优先级天然更高。**实测公共 pull-through 代理（nju 等）拉不了私有 GHCR package**（404 manifest unknown），因此仓内默认官方源；要用加速域需把 package 改 public，或走阿里云 ACR 反代（教程见 README）
- **pull_policy**：默认非强制 pull 提速启动；显式刷新和容器启动分离，心智清晰。local 保持 `$COMPOSE build`，和 `docker-compose.local.yaml` 里 `pull_policy: never` 一起保证永远用本地构建产物

**坑**:
- **同 tag 远端被重推**：`pull_policy: missing` 判定"镜像已存在"不拉，继续用旧镜像。v1.0.0 force-tag 场景必须 `./deploy.sh <env> pull` 显式刷新后再 up
- **防火墙/安全组**：服务器若放通过 9091/9092 入站，需改放通 9190/9290

**用法**:
```bash
# 按需切 ghcr 源（前提：package 已 public）
./deploy.sh prod --ghcr <mirror>
# 或改 .env.prod: GHCR_REGISTRY=<mirror>（永久）

# 同 tag 刷新要显式 pull 再 up
./deploy.sh prod pull && ./deploy.sh prod
```

**会话**: 260421-cicd-L1发版v1.0.0

## [2026-04-20] L1 部署体系：deploy/ 单源 + 一份 compose + 多 env 切换

**变更内容**: 完成 L1 部署 + CI 体系，最终落地的状态：

- `deploy/` 是部署唯一真相源（根目录原 `docker-compose.yaml` / `nginx/` / `prometheus/` / `grafana/` / `otel-collector/` 全部清空）
- `deploy/docker-compose.yaml`：一份 all-in-one（17 服务：webook + webook-fe + 中间件 + 监控栈 + exporters + nginx）
- `deploy/docker-compose.local.yaml`：override 文件，本地从 ghcr 改成本地 build + 暴露宿主端口
- `deploy/.env.{local,dev,staging,prod}(.example)`：四份 env 模板入库（占位 `CHANGE_ME`），真实 `.env.*` gitignored
- `deploy/deploy.sh <env>`：一键切换，`local|dev|staging|prod`，子命令 `up/down/nuke/logs/status/pull/restart`
- `deploy/nginx/conf.d/default.conf`：一份配置（通配 `server_name _`），不再按 env 拆
- `deploy/prometheus/` + `deploy/grafana/` + `deploy/otel-collector/`：完整 provisioning + examples（教学参考）
- `deploy/grafana/Makefile`：Grafana 运维命令（仪表盘导出/导入等）
- `deploy/README.md`：架构图 + 端口表 + 首次部署序列 + 日常操作

**多环境机制**:
- project 名按 env 分（`webook-dev` / `webook-prod`）→ volume 自然隔离 `webook-<env>_*`
- container_name 不带 env 前缀（`webook-app` / `webook-mysql`）→ docker 全局唯一，**同时只能跑一套**（切换前自动 stop）
- `APP_ENV` 由 `.env.<env>` 注入 → webook 加载对应 `config/<env>.yaml`
- 切环境 volume 不清，dev 数据切到 prod 再切回还在；显式清要 `nuke` 子命令

**配置体系（混合方案）**:
- `webook/config/{local,dev,staging,prod}.yaml` 同构按 env 差异化（otel.env / sampleRatio / logger 级别）
- 敏感字段（`mysql.dsn` / `redis.password`）yaml 占位 `OVERRIDE_VIA_ENV`，运行时由 `.env.<env>` 注入
- `webook/main.go` `initViperV2` 加 `viper.AutomaticEnv()` + `SetEnvKeyReplacer('.','_')` —— L2 K8s Secret 注入直接可用

**服务引用 K8s 心智对齐**:
- nginx / prometheus / grafana / otel-collector 跨服务引用全用 service name DNS（不写 IP）
- L2 K8s Service name 一一对应，提前练 K8s 思维

**CI 流水线**:
- `.github/workflows/webook-ci.yml` + `webook-fe-ci.yml`：main/feature push 触发，build → push ghcr，tag 含 `main-latest` 滚动 + 版本 tag
- `webook/Dockerfile` `ARG VERSION` + `LABEL org.opencontainers.image.version`，CI 注入版本元数据
- `paths-ignore '**.md'` 文档改不触发 CI

**影响范围**:
- `deploy/`（新建目录 + 完整文件集 ~25 文件）
- `webook/Dockerfile` / `webook/main.go`（CI 元数据 + AutomaticEnv 钩子）
- `webook/config/{local,dev,staging,prod}.yaml`（命名标准化 + 混合配置）
- `.github/workflows/webook-ci.yml` / `webook-fe-ci.yml`
- 根目录大量删除（旧 `docker-compose.yaml` / `nginx/` / `prometheus/` / `grafana/` / `otel-collector/`）
- `webook/CLAUDE.md` 加「环境说明」+「部署层」章节
- `.gitignore`（加 `!.env.*.example` 允许模板入库）

**技术决策**:
- 一份 compose + env 切换 vs 多 project 拆分 + 共享 infra：选前者，避免跨 project extnetwork 复杂度；故障域简单清晰
- 切 env 不清 volume：保留历史数据可随时切回；显式 nuke 才清
- 监控栈每环境重建：prometheus 数据单 env 独立不混淆，符合 L1「每 project 独立全套」哲学
- 服务引用用 service name 不用固定 IP：对齐 L2 K8s 心智模型（K8s 没人手写 IP）
- yaml 分环境 + env 注 secrets 的混合方案：PR review "改 dev 配置" 直接看 dev.yaml diff；微服务 N×M 不用维护 N×M env 变量
- prod tag 强校验语义化版本（`x.y.z`）：倒逼走 `git tag webook-v*` 流程，不允许 `main-<sha>` 上 prod
- example 用 `CHANGE_ME` 占位：强制部署者主动填密码，比示例写明文更安全

**待办**:
- L2：K3s + ArgoCD + Helm + Secret 分环境（AutomaticEnv 钩子已预埋，Secret 注入链路就绪）
- CI 接入集成测试（需 MySQL + Redis + Kafka + ES services，性能 / 成本另议）
- 实机验证：服务器 192.168.150.101 `git pull` + `./deploy.sh dev` 全 17 服务启动成功

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] GORM tracing 避开 AutoMigrate 启动噪音

**变更内容**: `webook/ioc/db.go` 把 `dao.InitTable(db)` 从 `tracing.NewPlugin` 注册之后调整到之前，让 AutoMigrate 的 `SELECT information_schema.statistics ...` 等系统表查询不再产生 `gorm.Query` / `gorm.Row` span。

**影响范围**:
- `webook/ioc/db.go`（仅调整 InitTable 与 tracing plugin 的注册顺序）

**技术决策**: gorm opentelemetry plugin 不支持按 database/table 过滤 span；`WithDBSystem` 只贴属性、不过滤。评估过 Collector 侧 filter processor（事后丢弃，浪费）、移除 Row callback（治标不治本）、换 uptrace/otelgorm（依赖变更大），最终选顺序调整——最小改动解决"启动期 information_schema 噪音"这个主要痛点。gormprom 保留在 InitTable 前，启动 SQL 仍会进 Prometheus metrics（聚合值影响忽略）。

**会话**: 260420-observability-采样率公式

## [2026-04-20] OTel Collector 升级 + 采样率公式文档化

**变更内容**: otel-collector 镜像 `0.88.0 → 0.105.0`；`webook/config/prod.yaml` 新增 `sampleRatio` 计算公式注释（`min(1.0, TargetTracesPerSec / AvgQPS)`）并在 `docs/opentelemetry/06-best-practices.md` 同步；`dev.yaml` 加公式引用注释。

**影响范围**:
- `docker-compose.yaml`（collector 版本）
- `otel-collector/config.yaml`（版本注释同步）
- `webook/config/prod.yaml` / `webook/config/dev.yaml`（采样率说明块）
- `docs/opentelemetry/06-best-practices.md`（新增「采样率计算公式」小节）

**技术决策**: 未选 0.116.0 因 CentOS 7 kernel 3.10 跑不起（`exec: no such file or directory`，实为新 Go runtime syscall 不兼容）；0.105.0 是经验证可运行的上限。采样率公式两处同步（配置 + 文档），注释明确标注"请勿删除"。

**待办**: 后续内核升级到 ≥5.x 或迁移 Rocky/Alma 9 后，可尝试升到 0.116+。

**会话**: 260420-observability-采样率公式

## [2026-04-19] 服务总览 dashboard + 监控栈自监控（Grafana/OTel/Zipkin）

**变更内容**: 新增 services-overview 大盘覆盖 up/主机/Go/MySQL/Redis/Kafka/监控栈/Zipkin 六大区；Prometheus 加 grafana/otel-collector/zipkin 三个 scrape job；OTel Collector 暴露 :8888 自监控；Zipkin 换 slim→full 以保留 /prometheus 端点

**影响范围**:
- `grafana/provisioning/dashboards/services-overview.json`（新）
- `prometheus/prometheus.yml`（+3 jobs，grafana 带 basic_auth）
- `otel-collector/config.yaml`（telemetry.metrics.address）
- `docker-compose.yaml`（zipkin 镜像换 full）

**技术决策**: grafana 显式带 basic_auth 兜底 metrics endpoint 可能 401；zipkin-slim 砍了 micrometer 所以换 full，+200MB 可接受

**会话**: 260419-ops-服务总览

## [2026-04-19] 配置模板参考体系 + Grafana/Prometheus examples 目录

**变更内容**: 为 `grafana/` 和 `prometheus/` 下每种配置文件建立完整注释的 `-example.yml` 模板 + 两份字段字典文档；Prometheus 启用 `--web.enable-lifecycle` 支持热 reload；Grafana dashboards 开启生产三件套

**影响范围**:
- 新增 `grafana/examples/`（独立目录，不被 Grafana 加载）：
  - `alerting/{contactpoints,policies,rules}-example.yml`（4 种告警范式、Alertmanager 模板语法、路由 + matcher + 静默时段）
  - `dashboards/{dashboards-example.yml, webook-example.json}`（provider 生产三件套 + 8 种 Panel 骨架 + 变量 + gridPos）
  - `datasources/{prometheus,zipkin,mysql,loki}-example.yml`（exemplar 联动、tracesToMetrics/Logs、只读账号、derivedFields）
  - `README.md` 对照表 + 使用流程
- 新增 `prometheus/examples/`：
  - `prometheus-example.yml`（global/scrape_configs/alerting/relabel/remote_write/多种 SD）
  - `alerts-example.yml`（Prom 原生告警规则 + recording rules + 模板变量说明）
  - `README.md` 解释两种告警方式（Prom 原生 vs Grafana Alerting）对比
- 新增文档：
  - `docs/grafana/08-alerting-template-reference.md`（Contact Point 邮件模板全字段字典 + Alertmanager template 语法 + 陷阱 cheatsheet）
  - `docs/grafana/09-provisioning-reference.md`（datasources/dashboards/rules/policies/Makefile 字段字典，定位为参考不是模板）
- 文档索引：`docs/grafana/README.md` + `docs/prometheus/README.md` 加"即开即用 example"板块
- 生产三件套落地：`grafana/provisioning/dashboards/dashboards.yml` 三件套全关（`disableDeletion:true editable:false allowUiUpdates:false`），UI 只读防误改
- Prometheus reload API 开启：`docker-compose.yaml` prometheus 服务加 `--web.enable-lifecycle`，`curl -X POST /-/reload` 即可热加载规则
- 模板修复（上轮未完全收敛）：
  - `grafana/provisioning/alerting/contactpoints.yml` 改成 Alertmanager template 语法（`.SortedPairs` 代替 `range $k,$v`）
  - `grafana/provisioning/alerting/rules.yml` 所有规则补 Reduce 节点，不再 "invalid format of evaluation results"
  - `otel-collector/config.yaml` verbosity 改回 basic（调试结束，detailed 日志量过大）
- 新增 `grafana/mk/grafana.mk`：Makefile 封装 reload / test-email / restart 命令

**技术决策**:
- `-example.yml` 文件单独放 `examples/` 不放 provisioning 下：Grafana 按扩展名加载，避免"测试模板被当真实数据源"；`.example` 后缀也能达到同样效果但 `-example.yml` + 独立目录更直观
- 文档 vs example 双份存在的定位：文档是"字段字典 + 坑速查"，examples 是"复制即用"。都改时以 examples 为准，文档只更新字段释义
- dashboards 生产三件套全关：UI 改动被重启覆盖会产生"诡异现象"（改完又消失），强制 Git 化唯一 source of truth

**待办**: 无
**会话**: 260419-template-reference-体系建设

## [2026-04-19] OTel 调用链完整串联 + Kafka 连接重试 + Grafana tracing dashboard

**变更内容**: 两个关键 bug 修复让 OTel trace 真正成为一棵完整的树；Kafka producer 启动竞态修复；Grafana 增加 tracing 入口 dashboard + 文档同步

**影响范围**:
- Bug 1：HTTP span 与 DB/Redis span 断成独立 trace
  - 根因：Gin 1.11 `*gin.Context.Value()` 默认不 fallback 到 `c.Request.Context()`
  - 修：`webook/ioc/web.go` `server.ContextWithFallback = true`
  - 测：`webook/ioc/web_ctx_propagation_test.go` 单测复现 true/false 两种行为
- Bug 2：Kafka producer 降级为 NoopProducer
  - 根因：webook 启动早于 Kafka JVM ready，`InitSaramaSyncProducer` 一次性失败返回 nil
  - 修（代码）：`webook/ioc/kafka.go` 新增 `retryConnect` 泛型工具，指数退避重试 6 次（共约 63s）；Producer + Client 都用
  - 修（编排）：`docker-compose.yaml` Kafka 加 healthcheck + webook `depends_on.kafka.condition: service_healthy`
  - 测：`webook/ioc/kafka_test.go` 3 用例（成功/耗尽/指数 backoff）
- Grafana tracing dashboard：`grafana/provisioning/dashboards/webook-tracing.json`
  - 5 个 panel：Text 说明 + QPS/错误率/P99/Goroutine Stat + HTTP QPS by path + 分位趋势
  - 顶部 link 跳 Zipkin UI + Webook/Overview
- Collector 版本：`otel/opentelemetry-collector-contrib:0.116.0` → `0.88.0`（CentOS 7 kernel 3.10 兼容）
- 文档同步：
  - `docs/opentelemetry/05-integration.md`：从"集成蓝图"改成"已落地 + 4 大踩坑记录"
  - `docs/opentelemetry/README.md`：加生产接入状态图（5 层 instrumentation + OTLP → Collector → Zipkin）
  - `docs/opentelemetry/06-best-practices.md`：踩坑清单追加 #13-16（Gin fallback / 启动竞态 / Collector kernel / IBM sarama）
  - `grafana/provisioning/dashboards/README.md`：新增"项目自有 Dashboard"表格

**技术决策**:
- ContextWithFallback=true 是 Gin + OTel 必选，单测保护防回归
- Kafka 重试走双保险：compose 层解决启动顺序，代码层解决运行期抖动
- retryConnect 抽成泛型工具，Producer/Client 共享，单测独立验证
- Collector 降到 0.88.0 兼容老 kernel，直接转发 OTLP → Zipkin，业务代码零改动

**生产验证（SSH）**:
- `Container webook-kafka Healthy` → webook 等 Kafka healthy 才起
- `docker logs webook-app | grep kafka` 空 → 无连接失败
- Zipkin API `?serviceName=webook&spanName=/article/reader/page` → 19 个 span 完整树形
- `[SERVER] /article/reader/page` 根下挂 `eval/get/select published_article×2/set/hgetall×10/select interaction`

**待办**: Service 层手动埋点（Article.Publish / Chat.Send）；Prometheus exemplar（Histogram observe 附 trace_id 实现大盘跳 trace）

**会话**: 260419-otel-fix-ctx-propagation+kafka-retry

## [2026-04-18] OpenTelemetry 全链路接入 + Grafana 邮件告警

**变更内容**: webook 集成 OpenTelemetry（OTLP/gRPC → otel-collector → zipkin），覆盖 HTTP 入口（otelgin）/ SQL（otelgorm）/ Redis（redisotel）；Grafana 接 Zipkin 数据源 + 配置 5 条核心告警 + QQ 邮箱 SMTP 通知

**影响范围**:
- OTel SDK 初始化：`webook/ioc/otel.go`（OTLP/gRPC exporter + Resource + ParentBased(TraceIDRatioBased) sampler + BatchSpanProcessor + W3C TraceContext propagator + wire cleanup）
- Wire 注入：`webook/wire.go` 加 `ioc.InitOTel`，签名改为 `(App, func(), error)`；集成测试 setup wire 加 `noop.NewTracerProvider`
- HTTP：`webook/ioc/web.go` `InitMiddlewares` 加 `tp trace.TracerProvider` 参数，挂 `otelgin.Middleware("webook", WithTracerProvider(tp))` 紧随 Prometheus 之后
- DB：`webook/ioc/db.go` `db.Use(tracing.NewPlugin(WithoutMetrics(), WithoutQueryVariables()))`
- Redis：`webook/ioc/redis.go` `redisotel.InstrumentTracing(client)`
- Kafka：`webook/ioc/kafka.go` 暂保留 TODO 注释（otelsarama 未适配 IBM/sarama）
- main：`webook/main.go` 处理 wire cleanup，进程退出 flush span
- 配置：`webook/config/{dev,prod}.yaml` 加 `otel` 块（dev → `localhost:4317`，prod → `otel-collector:4317`）
- 依赖：`webook/go.mod` 加 OTel SDK v1.32.0 + otlptracegrpc v1.32.0 + otelgin v0.57.0 + gorm.io/plugin/opentelemetry v0.1.16 + redisotel/v9 v9.18.0
- Mock 重生：`make -f mk/mock.mk mockgen`（go-redis 因 redisotel 升级，旧 mock 缺新方法）
- Collector：`otel-collector/config.yaml` 新增（OTLP receiver + memory_limiter + batch processor + zipkin exporter）
- Compose：`docker-compose.yaml` 加 `otel-collector` 服务（image `otel/opentelemetry-collector:0.116.0`，IP 172.21.0.29，port 4317）
- Grafana 数据源：`grafana/provisioning/datasources/zipkin.yml` 新增（uid=zipkin，关联 prometheus）；`prometheus.yml` 加 uid=prometheus 固定
- Grafana 告警：`grafana/provisioning/alerting/{contactpoints,policies,rules}.yml` 三件套
  - rules：5 条核心告警（服务 up / 5xx 率 > 1% / P99 > 500ms / Goroutine > 10000 / MySQL 连接 > 80%）
  - policies：按 alertname+severity 分组，critical 立即发，其它攒批 5m
  - contactpoints：邮件，收件人 `3236447743@qq.com`
- Grafana SMTP：`docker-compose.yaml` grafana 服务加 `GF_SMTP_*` 环境变量（QQ 邮箱 587 STARTTLS），凭证从 `.env` 注入
- 凭证管理：`.env`（本地真实，已 .gitignore）+ `.env.example`（模板，可入库）；`.gitignore` 加 `.env` `.env.*` 规则

**技术决策**:
- Exporter 选 OTLP/gRPC（CNCF 标准）+ otel-collector 中转：换后端只改 collector 出口，业务代码零改动；未来加 Tempo/Datadog 多 fan-out 也只动 collector
- Collector 用 core 版（80MB）而非 contrib（350MB）：仅需 OTLP receiver + zipkin exporter，core 全覆盖
- otelgin 紧随 Prometheus 之后：让所有下游中间件 / handler 都在 root span 上下文里
- Init 走 wire `(T, func(), error)` cleanup pattern：与项目既有 wire 风格一致，BatchSpanProcessor 队列退出时 flush
- Sampler 用 `ParentBased(TraceIDRatioBased)`：跨服务时跟随上游决策，避免一条 trace 一半在一半丢
- Kafka 暂未接 OTel：otelsarama 未适配 IBM/sarama，等社区或自己写包装；当前 producer/consumer span 缺失，TODO 标记
- gormopentelemetry 用 `WithoutMetrics()`：与现有 gormprom 互斥避免重复采集
- gormopentelemetry 用 `WithoutQueryVariables()`：SQL 参数可能含 PII，默认隐藏

**部署步骤（VM 上）**:
1. `git pull` 同步代码
2. `cp .env.example .env && vi .env` 填 SMTP 授权码（或 scp 已填好的 .env 过去）
3. `docker pull otel/opentelemetry-collector:0.116.0`（或国内镜像 retag）
4. `docker compose build webook && docker compose up -d`
5. 验证 trace：`curl http://192.168.150.101/api/user/login` → Zipkin UI `:9411` 看到 root span + DB/Redis 子 span；Grafana `:3001` Explore 选 Zipkin 也能查
6. 验证告警邮件：手动触发（如 stop webook → up == 0）等 1m 后收 QQ 邮箱

**待办**:
- Phase 3：service 层手动埋点（Article.Publish / Chat.Send / User.Login 等关键路径加 attribute / RecordError）
- Kafka OTel：自写 sarama producer interceptor / consumer middleware 注入 trace context 到 Kafka header
- Grafana exemplar：Prometheus Histogram observe 时附 trace_id，大盘高延迟点直跳 Zipkin

**会话**: 260418-otel-Phase1to5全链路接入

## [2026-04-18] 前后端 CI 完善 + GHCR 镜像推送 + nginx 同源部署

**变更内容**: 补齐前端 CI；前后端 CI 均扩展 `build-push` job 推镜像到 GHCR；docker-compose 新增 webook-fe + nginx 同源反代，实现浏览器只见一个域名、零 CORS
**影响范围**:
- 前端 CI 新建：`.github/workflows/webook-fe-ci.yml`（eslint + tsc + next build；main/feature/webook-fe-v 三种 tag 推 `ghcr.io/<user>/webook-fe`）
- 后端 CI 扩展：`.github/workflows/webook-ci.yml` 加 `build-push` job，先 `go build -tags=k8s` 再 docker build（保留简单 Dockerfile 策略）；tag `webook-v*.*.*` 也触发
- 前端 Dockerfile：`webook-fe/Dockerfile` npm registry 改 `ARG NPM_REGISTRY` 可传参（默认官方源，CI 用默认，本地按需切 npmmirror）
- 部署栈：`docker-compose.yaml` 加 `webook-fe`（3000 仅 expose）+ `nginx`（80 对外，同源反代：`/` → 前端，`/api/*` → 后端）
- nginx 配置：`nginx/nginx.conf` + `nginx/conf.d/webook.conf`（upstream + 安全头 + healthz + 代理参数）
- Workflow 完善：两个 workflow 加 `workflow_dispatch`（可手动触发）和顶层 `permissions: contents: read`
**技术决策**:
- 同源部署：前端 `NEXT_PUBLIC_API_BASE_URL=/api`（相对路径）**一次构建多环境通用**，不用再为不同 API 地址构建多个镜像
- Workflow `paths` 过滤移除：否则 tag push 指向的 commit 若 diff 不含目标目录会静默不触发，发版失败
- 后端不改 Dockerfile 走简单单阶段：与项目既有决策一致（commit 36e8dbe 回归精简），构建责任在 CI 外部
- npmmirror 改成构建参数而非硬编码：CI 走官方源避免海外 runner 访问国内镜像站失败
**待办**: GitHub 仓库 Settings → Actions → Workflow permissions 勾选 "Read and write permissions" 否则 GHCR 推送 403；设置分支保护强制 PR + CI 绿
**会话**: 260418-infra-CI完善前端部署

## [2026-04-18] 学习沙箱归档到 sandbox/

**变更内容**: `work/` 顶层散落的 8 个独立 Go 学习模块统一归档到 `sandbox/`，主目录只剩项目核心（webook / webook-fe / 基础设施 / docs）
**影响范围**:
- 目录归并（git mv 保留历史）：`context/` `gin/` `gorm/` `mongodb/` `opentelemetry/` `sarama/` `syntax/` `wire/` → `sandbox/<name>/`
- 文档同步：`docs/opentelemetry/` 共 7 处 `opentelemetry/` 路径引用 → `sandbox/opentelemetry/`
**技术决策**:
- 命名 `sandbox/` 而非 `learning/`：中性专业，不带学生味
- 扁平结构（不按主题分组）：沙箱本来就孤立，分类反而增加查找深度
- 各沙箱保留独立 go.mod，依赖树不互相污染
**待办**: 无
**会话**: 260418-refactor-沙箱归档

## [2026-04-18] 学习沙箱：context + opentelemetry trace

**变更内容**: 新增两个独立 Go 学习沙箱
**影响范围**:
- `sandbox/context/`（独立模块 `context-demo`，演示 WithValue/WithCancel/WithTimeout/父子传导/反向隔离，5 个测试）
- `sandbox/opentelemetry/`（独立模块 `otel-demo`，OTel SDK v1.32.0，stdout + Zipkin 双 exporter 测试）
**技术决策**: 独立 go.mod 与主模块隔离，依赖树不互相污染；与 `sandbox/mongodb/` `sandbox/sarama/` `sandbox/gin/` `sandbox/gorm/` `sandbox/wire/` `sandbox/syntax/` 等已有学习沙箱风格一致
**待办**: 无
**会话**: 260418-learning-context+OTel

## [2026-04-18] 前端 Docker 部署 + Nginx 反代 + 基础设施目录顶层化

**变更内容**: webook-fe 多阶段 Docker 镜像（standalone + 非 root + healthcheck + OCI labels）；Nginx 反向代理（同源部署 `/api` 剥前缀、SSE 关 buffering、登录强限流、静态资源长缓存、安全响应头）；docker-compose.yaml / nginx / prometheus / grafana 全部上移到 `work/` 顶层，`webook/` 目录回归"纯后端代码"
**影响范围**:
- 前端镜像：`webook-fe/Dockerfile`（多阶段 deps→builder→runner，standalone 输出，非 root nextjs:1001，HEALTHCHECK，OCI 标签）+ `webook-fe/.dockerignore` + `webook-fe/next.config.ts`（`output: 'standalone'` + `poweredByHeader: false` + `compress: true`）
- 反代：`nginx/nginx.conf`（JSON access log、gzip、limit_req zone）+ `nginx/conf.d/webook.conf`（upstream + 安全头 + SSE 特殊路径 + metrics 内网白名单 + Next 静态长缓存）
- 编排：`docker-compose.yaml` 加 `webook-fe`（仅 expose 3000）+ `nginx`（80 对外，不挂日志卷以保留 `/dev/stdout` 软链让 docker logs 接管）；启动命令注释更新
- 目录顶层化（git mv，保留历史）：`webook/docker-compose.yaml` → `docker-compose.yaml`、`webook/prometheus/` → `prometheus/`、`webook/grafana/` → `grafana/`
- 文档同步：`docs/grafana/` `docs/prometheus/` 共 4 处路径引用从 `webook/grafana` → `grafana` 等
**技术决策**:
- 同源部署：`NEXT_PUBLIC_API_BASE_URL=/api`，构建一次到处部署，无 CORS，外暴露端口仅 80
- standalone 输出：镜像最小化，仅装运行时 node_modules
- nginx 在容器网络内部通过 upstream `webook:8090` / `webook-fe:3000` 拼接，无需暴露后端端口（保留 8090 暂供调试）
- 顶层化：`webook/` 不再承担"基础设施 + 后端"双重职责，`nginx/prometheus/grafana` 是跨前后端基础设施，与 `webook/` 平级更清晰
**待办**: 后端去掉 8090 公网端口（仅走 nginx）；TLS（443）+ HSTS；webook 接 OTel；Grafana provisioning 切 `editable=false` 三件套
**会话**: 260418-deploy-前端Docker部署

## [2026-04-18] Grafana / OpenTelemetry 文档 + Zipkin 接入

**变更内容**: 新增 Grafana / OpenTelemetry 详细学习文档（共 13 篇），docker-compose 引入 Zipkin 服务（暂用内存存储），原 `docs/monitoring/` 重命名为 `docs/prometheus/` 与新文档对齐
**影响范围**:
- 文档：`docs/opentelemetry/`（概念/Tracing 模型/上手/Exporter 选型/接入 webook/最佳实践，6 篇）
- 文档：`docs/grafana/`（概念/部署/数据源/Dashboard/告警/生产级流程/最佳实践，7 篇）
- 文档重命名：`docs/monitoring/` → `docs/prometheus/`（git mv 保留历史，同步修复 grafana/otel 文档内引用）
- 基础设施：`docker-compose.yaml` 加 `zipkin`（openzipkin/zipkin:3.4，9411，IP 172.21.0.26，mem 存储 + ES 切换注释占位 + healthcheck）
**技术决策**:
- 文档目录三件套对齐（prometheus / grafana / opentelemetry），命名一致便于检索
- Zipkin 暂内存存储，重启丢 trace 数据；切 ES 路径以注释保留（复用 `webook-es`），单节点 ES 必须 `ES_INDEX_REPLICAS=0`
**待办**: webook 接入 OTel（otelgin/otelgorm/otelredis/otelsarama）；接入后切 OTLP exporter；按需切 Zipkin ES 存储
**会话**: 260418-observability-OTel起步

## [2026-04-17] GitHub 迁移 + CI 体系

**变更内容**: 仓库从 gitee 迁至 GitHub（`github.com/boyxs/train-go`），Go 模块路径改为 `github.com/webook`，落地 GitHub Actions CI（goimports + vet + test），handler 测试适配 ginx.WrapReq HTTP 状态码映射
**影响范围**:
- 模块路径：`webook/go.mod` + 132 个 .go 文件 import 全替换（`gitee.com/train-cloud/geektime-basic-go` → `github.com/webook`）
- Import 规范：`goimports -local github.com/webook` 重排 111 个文件，本地包分组到第三组
- Makefile：`webook/Makefile` 加 `fmt` / `fmt-check` target，MODULE 从 go.mod 动态提取
- CI：`.github/workflows/webook-ci.yml`（lint-test：goimports 检查 + go vet + go test -race），actions/checkout@v5、setup-go@v6（消除 Node 20 警告）
- Test 修复：`internal/web/` 5 个 test 函数的 wantCode 按 ginx.WrapReq 状态码映射校正（Code 4→400、Code 5→500）；`internal/service/sms/tencent/` TestSend 无凭证时改 `t.Skip()`
- Remote：origin → GitHub，原 gitee 重命名为 `gitee` remote 保留
**技术决策**:
- 模块路径采用 `github.com/webook` 而非 `github.com/boyxs/train-go/webook`：短路径更简洁，私有项目不需要外部 `go get`
- CI 不依赖 make，直接调 goimports 二进制：降低 runner 环境耦合
- 当前只有 lint-test job，build-push 留到 L1 完整流程时加（需配合 Dockerfile 多阶段）
**待办**: Dockerfile 改多阶段 + CI 加 build-push → GHCR；打开 GitHub 仓库分支保护（main 强制 PR + CI 绿）
**会话**: 260417-infra-GitHub迁移

## [2026-04-14] Prometheus 监控链路

**变更内容**: HTTP 指标中间件 + Prometheus/Grafana/Exporter Docker 栈 + 3 个自定义 Grafana 面板 + PromQL 文档
**影响范围**:
- 中间件：`pkg/ginx/middleware/metrics/`（Builder 接口 + PrometheusBuilder 实现，4 种指标按需启用）
- 集成：`ioc/web.go`（/metrics 端点 + 中间件注册）
- Docker：`docker-compose.yaml`（prometheus/grafana/mysqld-exporter/redis-exporter/kafka-exporter/node-exporter）
- 配置：`prometheus/prometheus.yml`、`prometheus/prometheus.local.yml`、`grafana/provisioning/`
- 面板：`webook-overview.json`（17 面板）、`webook-ops.json`（15 面板）、`linux-host.json`（18 面板）
- 文档：`docs/monitoring/`（架构/部署/PromQL/实战查询/告警/最佳实践，6 篇）
- 目录整理：`docs/` PRD 文件移至 `prd/`，`docs/` 改为技术文档目录
- 配置同步：`config/prod.yaml` 补齐 llm/embedding/ollama 配置
**技术决策**: 中间件用 Builder 接口抽象，Prometheus 为具体实现，后续可换 OpenTelemetry；Histogram + Summary 同时开启便于学习对比，生产建议二选一
**待办**: 生产环境 API Key 移到密钥管理
**会话**: 260414-monitoring-Prometheus监控

## [2026-04-12] AI 回复操作栏（复制 + 点赞/点踩）

**变更内容**: AI 消息气泡底部增加操作栏，支持复制回复内容、点赞/点踩反馈
**影响范围**:
- 后端：`domain/chat.go`（Message +Feedback）· `dao/chat_message.go`（+Feedback 字段 + UpdateFeedback）· `repository/chat_message.go`（UpdateFeedback + 清缓存）· `service/chat.go`（SetFeedback 归属校验）· `web/chat.go`（/chat/message/feedback 路由）
- 前端：`types/chat.ts`（+feedback）· `api/chat.ts`（setMessageFeedback）· `hooks/useChat.ts`（乐观更新 setFeedback）· `views/chat/MessageBubble.tsx`（ActionBar 组件）· `ChatMessages/ChatBubble/index.tsx`（props 传递）
- 原型：`prd/chat/chat.pen`（06-消息反馈画板）· `prd/chat/prototypes/06-消息反馈.png`
- 测试：8 个集成测试（点赞/取消/点踩/无效值/无效ID/非属主/幂等/列表展示）
**技术决策**: feedback 字段直接加在 message 表（1:1 关系），不新建表；前端乐观更新用函数式 setState 避免闭包竞态
**待办**: 无
**会话**: 260412-chat-消息反馈

## [2026-04-11] Chat SSE 断线续传（Redis Stream）

**变更内容**: 用 Redis Stream 实现 SSE 断线续传，刷新/切换对话后从断点继续流式输出
**影响范围**:
- 后端：`pkg/streamer/`（EventStreamer 接口 + Redis Stream 实现）· `service/chat.go`（publishToStream 写入 + ReadStream/BlockReadStream 读取 + isGenerating 内存标记）· `web/chat.go`（ResumeStream GET SSE 端点 + pollStream + formatSSE）· `consts/cache.go`（ChatStreamPattern + ChatStreamTTL）
- 前端：`api/chat.ts`（resumeStream SSE 重连函数）· `hooks/useChat.ts`（buffer 管理 + isStale 守卫 + resumeStream 恢复）
**技术决策**:
- Redis Stream 做消息中转，`XREAD BLOCK` 零空转阻塞读
- Stream 生成完成后 5 分钟 EXPIRE，供断线重连
- 前端 `isStale()` 守卫所有异步回调，防止快速切换对话时数据串
- 有活跃 buffer 时 effect 直接 return，避免与 send() 竞态
**会话**: 260411-SSE-Redis-Stream

## [2026-04-11] 文章润色助手 + 原型配色统一 + Chat 生成不中断 + 阅读量展示

**变更内容**: 文章润色助手（LLM 同步调用 + 对比 Modal）；原型配色全量统一为 teal #0D9488；Chat 生成独立 context 不被刷新/切换中断，支持刷新后轮询恢复；阅读页互动栏对齐原型（独立卡片 + 点赞收藏按钮样式）；文章广场/作者列表新增阅读量展示；首页按钮对齐原型；Header logo 统一主色；代码块语法高亮
**影响范围**:
- 后端：`service/article_polish.go`（润色服务）· `web/article_polish.go`（Handler + 限流）· `service/ai/llm.go`（LLMClient.Chat 同步方法）· `service/ai/openai.go` + `failover.go` + `timeout_failover.go`（Chat 实现）· `service/chat.go`（独立 context + placeholder + onFlush + isGenerating + trySend 非阻塞）· `web/chat.go`（isGenerating 接口）· `web/jwt/handler.go`（CheckSession Redis 容错）· `domain/chat.go`（ArticleCard.Url）· `chat_tools.go`（工具结果带 url）
- 前端：`views/article/read.tsx`（互动栏重构）· `views/article/feed.tsx`（阅读量）· `views/article/list.tsx`（阅读量列 + 移动端卡片重构）· `views/article/edit.tsx`（AI 润色按钮 + PolishModal）· `views/home.tsx`（按钮对齐原型）· `views/chat/index.tsx`（Header 导航重构）· `views/chat/MessageBubble.tsx`（工具卡片下移 + 语法高亮）· `hooks/useChat.ts`（buffer 缓存 + 轮询恢复）· `components/layout/Header.tsx`（logo 主色）
- 原型：3 个 pen 文件配色统一 + PNG 重新导出 + PRD 更新
**技术决策**:
- Chat 生成用 `context.WithTimeout(context.Background(), 2min)` 独立于 HTTP 请求
- placeholder 消息预插入 DB + onFlush 每 2 秒更新，支持刷新后轮询看到进度
- trySend 加 default 分支，channel 满时不阻塞（浏览器断开后生成继续）
- isGenerating 用内存 sync.Map，不依赖 Redis
- 切换对话用 bufferRef 缓存 pending 消息，切回来立即恢复
- CheckSession Redis 出错时容错放行，解决间歇性点赞状态丢失
**待办**:
- Chat SSE 断线续传：用 Redis Stream 替代轮询，支持 Last-Event-ID 断点续传（为微服务拆分做准备）
- P1 文章标签/分类体系 → 意图识别 + 路由分发
- P2 用户反馈（点赞/点踩单条回复）
**会话**: 260411-原型实现-润色-Chat优化

## [2026-04-10] AI 点击埋点 + 数据看板 + Chat 工具调用修复

**变更内容**: 新增 AI 文章点击埋点全链路（记录 + 看板 + 缓存），修复 Chat Function Calling SSE 事件嵌套层级、工具结果刷新丢失问题，代码块语法高亮，命名规范统一
**影响范围**:
- 后端：`web/click_event.go`（Handler）· `service/click_event.go` · `repository/click_event.go` · `dao/click_event.go` · `cache/click_event.go`（全链路新增）· `service/chat.go`（saveReply 持久化 toolResults、buildPrompt 用 ListRecentLite）· `service/ai/openai.go`（finish_reason 兼容）· `service/chat_tools.go` + `chat.go`（命名 AIChatService/AIChatToolExecutor）
- 前端：`api/chat.ts`（SSE data.data 修复）· `hooks/useChat.ts`（历史消息还原 toolStates）· `views/chat/MessageBubble.tsx`（语法高亮 + 链接埋点）· `components/chat/ArticleCardBlock.tsx`（卡片埋点）· `views/dashboard/ai.tsx`（看板页面）· `views/article/read.tsx`（返回按钮兼容新标签）
**技术决策**:
- 命名规范：接口通用名，实现用 `{技术}{业务}{领域}{层}` 组合前缀（如 `CacheAIClickEventRepository`）
- 去重用 `uk_dedup`（user+article+conversation+source），`ON CONFLICT DO NOTHING`
- 看板缓存 10min TTL + jitter，写入后清缓存
- `ListRecentLite` 排除 tool_calls 字段优化 buildPrompt 性能
- 工具结果序列化存入 message.tool_calls，前端加载历史时还原卡片
**待办**: Phase 6 意图识别 + 路由分发（FAQ / RAG / Tool）
**会话**: 260410-chat-埋点看板

## [2026-04-06] Chat Function Calling — 工具调用 + 热门文章精确排行

**变更内容**: Chat 接入 OpenAI Function Calling，实现三个工具（文章搜索、热门推荐、我的收藏）；热门文章改为按互动加权分（read×1 + like×3 + collect×5）真实排行；前端渲染工具执行状态和文章卡片
**影响范围**: `service/ai/llm.go` · `service/ai/openai.go`（streaming tool_call 拼接）· `service/chat.go`（runStream 多轮工具循环）· `service/chat_tools.go`（新增 ToolExecutor）· `repository/dao/interaction.go`（ListHotBizIds / ListCollectedBizIds）· `domain/chat.go`（ArticleCard / ToolResultData）· `wire.go` · `hooks/useChat.ts` · `components/chat/ArticleCardBlock.tsx` · `views/chat/MessageBubble.tsx`
**技术决策**: 工具调用循环最多 5 轮防无限循环；executor 为 nil 时降级（兼容无工具场景）；热门排行加权而非最新，`stream_options include_usage=true` 修复 token 用量归零问题；system prompt 规则 8 强制每次重新调用动态数据工具
**待办**: Phase 6 意图识别 + 路由分发（FAQ / RAG / Tool）

## [2026-04-06] Chat RAG — 基于文章内容的知识问答

**变更内容**: 聊天接入 RAG，用户提问时自动检索平台文章，将相关内容注入 prompt，AI 基于文章回答并附带可点击链接
**影响范围**: `service/chat.go`（RAG 逻辑）· `wire.go`（DI 注入 ArticleSearchService）· `views/chat/MessageBubble.tsx`（链接新标签页打开）· `views/article/read.tsx`（返回按钮兼容新标签页）· `.gitignore`（排除 .next 构建产物）
**技术决策**: 复用已有 ArticleSearchService（hybrid BM25 + 向量），top-3 召回注入 system message；直接提供完整 Markdown 链接防止 LLM 编造 ID；检索失败静默降级不阻塞对话
**待办**: 阶段 B Function Calling（search_articles / list_favorites / hot_articles）
**会话**: 260406-chat-RAG

## [2026-04-05] Embedding 分包 + 分页状态保持

**变更内容**: ai/embedding 拆为独立子包，缓存移到 cache 层；文章列表分页同步 URL，编辑返回不丢页码
**影响范围**: `service/ai/embedding/`（新子包）· `cache/embedding.go`（缓存归位）· `views/article/list.tsx` · `views/article/edit.tsx`
**技术决策**: 子包按能力拆分（LLM vs Embedding），接口用完整名 `EmbeddingClient` 避免歧义；分页用 URL searchParams 而非 state，支持刷新保留

## [2026-04-05] 本地 Ollama Embedding + 收费模型降级

**变更内容**: 向量化优先走本地 Ollama bge-m3，失败自动降级到阿里百炼收费 API
**影响范围**: `ai/ollama_embedding.go` · `ai/failover_embedding.go` · `ioc/es.go` · `config/`
**技术决策**: 复用 `EmbeddingClient` 接口 + `FailoverEmbeddingClient` 顺序尝试，外层 `CachedEmbeddingClient` 不变
**会话**: 260405-embedding-ollama-failover

## [2026-04-05] 搜索优化 + 时间 int64 统一 + 性能修复

**变更内容**: 搜索功能全链路优化、时间字段统一为 int64 毫秒时间戳、多项性能和缓存修复
**影响范围**: DAO/domain/repository/service/web 全层 + 前端 types/views + ES mapping + SQL schema
**技术决策**:
- 时间用 int64 而非 time.Time：消除时区歧义，全链路零转换，前端 dayjs 格式化
- 搜索用 script_score 替代顶层 kNN：避免 kNN OR 合并返回全部文档
- Embedding 加 Redis 缓存（装饰器模式）：相同查询不重复调 API
- Wire/Mock 禁止手动改生成文件，统一用命令生成
**待办**: FindByBizIds 批量缓存、ES kNN+RRF 混合搜索（需 ES 8.8+）
**会话**: 250404-搜索优化-时间统一

## [2026-04-04] 安全修复：Chat 越权删除消息 + 越权取消生成

**变更内容**: 修复 3 处安全 / 架构问题
**影响范围**: `dao/chat_conversation.go` · `service/chat.go` · `service/chat_test.go`
**技术决策**:
1. `Delete` 事务先删 Conversation（uid 校验 + RowsAffected）再删 Messages，防越权删他人消息
2. `cancel` map 改用 `uid:convId` 复合 key，防任意用户 `/chat/stop` 取消他人生成
3. `isNotFound` 改用 `repository.ErrRecordNotFound`，service 层不再直接依赖 GORM
**会话**: 260404-chat-security-fix

## [2026-04-04] AI 客服多 LLM 故障转移 + 项目治理

**变更内容**: 新增 Kimi LLM 提供方，实现 FailoverClient（轮询）+ TimeoutFailoverClient（粘性超时切换）双策略故障转移；前端 SSE 超时断开 + 业务错误即断；consts 按领域拆分；CLAUDE.md 精简为导航+规则
**影响范围**: `service/ai/`（新增 4 文件）、`service/chat.go`（forwardStream 防泄漏重写）、`config/types.go`、`ioc/chat.go`、`consts/`（拆 3 文件）、`api/chat.ts`、`hooks/useChat.ts`、`CLAUDE.md` × 3
**技术决策**: 参考 SMS failover 拆为两种独立策略（单一职责），不合并；超时计入故障但不轮询（网络差时备用也超时）；业务错误不计入 cnt（切 provider 也没用）；forwardStream 所有 channel 写入改为 select ctx.Done 防 goroutine 泄漏
**待办**: 集成测试 SSE 用例需要有效 API Key 才能跑通
**会话**: 240404-全栈-AI客服故障转移

## 2026-03-29

### 读者端首页分页缓存 + 代码质量修复

**变更内容**:
- 读者端分页首页缓存（方案 C：只缓存第一页，TTL 3min）
- DAO 层 Insert/Update/Upsert 自动补充 abstract（`ensureAbstract`）
- 互动模块：logger 注入替代 `zap.L()`、事务包裹、错误码补充
- 前端 `Modal.confirm` → `modal.confirm`（antd 规范）
- SQL 补充 abstract 列和数据
- mock.mk 修复 article mock 生成路径

**影响范围**: `repository/cache/article.go`、`repository/article_reader.go`、`dao/article_author.go`、`dao/article_reader.go`、`dao/interaction.go`、`repository/interaction.go`、`web/interaction.go`、`webook-fe/views/article/list.tsx`、`scripts/webook.sql`

### 点赞/收藏功能（第2期互动模块）

**变更内容**: 新增文章互动功能，包含阅读量上报、点赞、收藏，前后端全栈实现
**影响范围**:
- 后端：domain/interaction.go（新），dao/interaction.go（新，`interaction`+`user_interaction` 两张表），repository/interaction.go（新），service/interaction.go（新），web/interaction.go（新），ioc/web.go，wire.go，wire_gen.go，init_table.go
- 前端：types/interaction.ts（新），api/interaction.ts（新），views/article/read.tsx（更新），types/index.ts（更新）

**新增接口**:
- `POST /interaction/read` — 阅读上报（公开，无需登录）
- `POST /interaction/get` — 获取聚合计数+用户状态（公开，登录后有 liked/collected 状态）
- `POST /interaction/like` — 点赞/取消点赞（需登录）
- `POST /interaction/collect` — 收藏/取消收藏（需登录）

**技术决策**:
- 聚合计数独立表 `interaction`（article_id unique），避免污染 published_article
- 用户操作记录独立表 `user_interaction`（user_id+article_id 联合唯一索引）
- 计数用 `GREATEST(0, count + delta)` 防止下界越零
- 前端乐观更新：点赞/收藏立即更新本地状态，失败时显示错误提示
- 阅读上报在文章内容加载完成后触发，失败静默处理不影响阅读体验
- `/interaction/read` 和 `/interaction/get` 加入 JWT 白名单（允许匿名访问）

**会话**: 260329-like-collect

---

## 2026-03-26

### 文章发布/撤回功能

**变更内容**: 实现文章发布（制作库+线上库双表事务）和撤回（幂等，只对已发布文章改状态）功能
**影响范围**: domain/article.go, dao/article_author.go, dao/article_reader.go, repository/article_author.go, service/article.go, web/article.go, wire.go, integration/article_test.go
**技术决策**:
- 制作库/线上库用 author/reader 命名，发布事务合并到 ArticleAuthorDAO（作者的操作）
- 撤回采用幂等设计：已发布→Private，草稿/已撤回→不改状态，线上库 DELETE 幂等
- DAO 层不依赖 domain 包，状态常量只在 domain 定义一份，通过 Repository 传参
- ArticleReaderDAO 暂时只保留 PublishedArticle 模型，接口和实现待未来读取功能时添加
**待办**: Publish 接口缺少"发布不存在文章"测试用例
**会话**: 260326-article-publish

---

## 2026-03-25

### 学习笔记：第01-03周

生成专业学习笔记，覆盖 Go 基本语法、Gin/GORM 入门、Session/JWT 认证、Kubernetes 部署入门。按知识体系重新组织，补充底层原理、最佳实践、对比表格和面试要点。

**文件:** `notes/_sections/sec_01_03.md`（2400+ 行）

---

## 2026-03-08

### 微信 OAuth2 登录

微信扫码登录完整流程：AuthURL 生成授权链接 → 回调获取 code → 换取 access_token + openid → 查找/创建用户 → 签发双 Token

**文件:** `internal/web/wechat.go` / `internal/service/oauth2/wechat/service.go` / `internal/domain/wechat.go` / `ioc/wechat.go`

### JWT 双 Token 认证（长短 Token）

access token 30min + refresh token 7天，refresh 时 ssid 不变只重签 access token，logout 时 Redis 存 ssid 使会话失效

**文件:** `internal/web/jwt/handler.go` / `internal/web/jwt/types.go` / `internal/web/user.go`

### SMS 装饰器：权限控制 / 限流 / 故障转移

SMS 服务支持多种装饰器组合：JWT 权限校验(`sms/auth/`) / Redis 滑动窗口限流(`sms/ratelimit/`) / 轮询故障转移(`sms/failover/`) / 连续超时切换(`sms/failover/timeout_failover.go`)

### 集成测试框架

Wire 独立 setup + MySQL/Redis 容器化测试环境，覆盖用户注册/登录/微信登录等核心流程

**文件:** `internal/integration/` / `internal/integration/setup/`

## 2026-03-15

### Docker Compose 本地开发环境

从 K8s deployment 文件中提取 MySQL 8.0、Redis 7.0、etcd 3.5.17 三个基础设施服务，生成 `docker-compose.yaml`，方便本地 `docker compose up -d` 一键启动开发依赖

**文件:** `docker-compose.yaml`

### Logger 中间件

请求日志中间件，Builder 模式构建，记录 Path/Query/Method/ClientIP/UserAgent/ReqBody/ResBody/Status/Duration，ReqBody 和 ResBody 截断 2048 字节

**文件:** `internal/web/middleware/logger.go`

### Logger 配置模块

基于 viper 配置文件路径判断环境（dev → DevelopmentConfig，k8s → ProductionConfig），通过 `LoggerConfig` 中间结构体将 yaml 配置逐字段覆盖到 `zap.Config`，解决 `viper.UnmarshalKey` 无法反序列化 `AtomicLevel`/`EncoderConfig` 的问题

**文件:** `ioc/logger.go` / `config/dev.yaml` / `config/k8s.yaml`

### Docker Compose webook 服务

webook 服务加入 docker-compose，传递 `APP_ENV=config/k8s.yaml` 环境变量，Dockerfile 增加 COPY config 目录

**文件:** `docker-compose.yaml` / `Dockerfile`

### 全局日志替换

项目中所有 `log.Printf`/`log.Println`/`fmt.Printf` 调试打印替换为注入式 `logger.LoggerX`，通过构造函数注入而非全局变量，涉及 user handler、failover SMS、memory SMS、ratelimit builder、login middleware。补充 `logger.Int32`/`Uint64`/`Strings` 辅助函数

**文件:** `internal/web/user.go` / `internal/service/sms/failover/` / `internal/service/sms/memory/` / `pkg/ginx/middleware/ratelimit/builder.go` / `internal/web/middleware/login.go` / `pkg/logger/fields.go`

### 集成测试 viper 初始化

添加 `TestMain` 入口加载 `config/dev.yaml`，修复集成测试无法连接数据库的问题

**文件:** `internal/integration/init_test.go` / `internal/integration/setup/wire.go`

## 2026-03-17

### 时间类型改造：int64 → time.Time

DAO 层和 Domain 层的 `Birthday`/`CreatedAt`/`UpdatedAt` 从 `int64` 毫秒时间戳改为 `time.Time`，MySQL 存储从 `BIGINT` 改为 `DATETIME`，和 MongoDB BSON Date 行为一致

**策略:** 全链路 UTC 存储 + carbon 东八区查询

**配置:**
- DSN: `parseTime=true&loc=UTC` — MySQL 驱动按 UTC 解析 datetime
- `ioc/time.go`: `carbon.SetTimezone("Asia/Shanghai")` — carbon 全局东八区
- `main.go`: 启动时调用 `ioc.InitTimezone()`

**时区处理规则:**
- 存储: 前端传毫秒时间戳 → `time.UnixMilli(ts).UTC()` → 存入 MySQL datetime（UTC）
- 查询: `carbon.Now().StartOfDay().StdTime()` 自动按东八区算一天的开始/结束
- 展示: 前端 `dayjs()` 自动将 UTC 转本地时区显示

**踩坑:**
1. `time.Time` JSON 反序列化只认 RFC3339，前端传时间戳必须用 `int64` 接收
2. `dayjs(value, 'YYYY-MM-DD')` 带格式参数会丢时区信息导致日期少一天，应用 `dayjs(value)`
3. carbon 默认用 UTC，必须 `SetTimezone` 后 `StartOfDay()` 才是东八区零点

**文件:** `ioc/time.go` / `ioc/db.go` / `config/dev.yaml` / `config/prd.yaml` / `internal/repository/dao/user.go` / `internal/domain/user.go` / `internal/domain/article.go` / `internal/repository/user.go` / `internal/web/user.go`
