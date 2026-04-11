# CHANGELOG

<!-- 新功能前插在此，日期降序 -->

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
