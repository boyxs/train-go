# 评论功能架构设计

> 配套需求见同目录 `PRD.md`，原型见 `comment-*.png`。
> 服务：webook 评论微服务（`webook/comment/`，gRPC server）+ webook-core（HTTP 网关 + comment gRPC client + interaction 聚合）+ 前端。

## 0. 调用拓扑（事实推导，非臆测）

```
前端 → nginx → core HTTP /api/comment/*  ──(gRPC, etcd 服务发现)──▶  webook-comment (gRPC server :8031)
                     │                                                     │
              聚合/鉴权/VO 转换                                   service → repository → dao
                     │
              core 内部 interaction service（同进程）── 评论点赞 / 计数 / liked（biz="comment"）
```

依据：① `webook/internal/grpc/` 全是 server 实现，core 对外暴露 gRPC；② nginx 无 comment 路由、core ioc 无 comment 引用 → comment 不直连前端；③ 与 article/interaction/search 对称——**core 是唯一面向前端的 HTTP 网关，comment 是纯 gRPC 后端**。comment 的 HTTP :8030 仅 metrics/health。

- **core 侧新增**：comment gRPC client 并入 `internal/ioc/grpc.go`（`CommentConn`/`InitCommentClient`，与 gRPC server 共享 `InitGRPCMetrics` 单例避免指标重复注册）+ `internal/web/comment.go`（HTTP handler，聚合 comment + interaction）+ wire
- **comment 侧填充**：proto / dao / repository / service / grpc server / ioc.InitGRPCServer + wire

## 1. 数据设计

### 1.1 表结构（仅 `comment` 一张；点赞数据在 interaction，不在本服务）

```sql
DROP TABLE IF EXISTS `comment`;
CREATE TABLE `comment` (
  `id`         bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz`        varchar(32)   NOT NULL DEFAULT ''     COMMENT '业务类型：article（P0 仅此）',
  `biz_id`     bigint        NOT NULL DEFAULT 0      COMMENT '业务对象 ID（文章 ID）',
  `uid`        bigint        NOT NULL DEFAULT 0      COMMENT '评论者用户 ID',
  `root_id`    bigint        NOT NULL DEFAULT 0      COMMENT '根评论 ID（一级评论=0；回复继承祖先 root_id）',
  `pid`        bigint                 DEFAULT NULL   COMMENT '父评论 ID（一级评论为 NULL；自关联外键）',
  `content`    varchar(1000) NOT NULL DEFAULT ''     COMMENT '评论内容（业务限 ≤500 字）',
  `reply_cnt`  bigint        NOT NULL DEFAULT 0      COMMENT '回复数：一级评论=整楼回复数（写/删回复增减楼根）；楼内回复恒 0',
  `created_at` bigint        NOT NULL DEFAULT 0      COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint        NOT NULL DEFAULT 0      COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint        NOT NULL DEFAULT 0      COMMENT '软删除时间（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_comment_biz_root` (`biz`, `biz_id`, `root_id`, `id`) USING BTREE,
  INDEX `idx_comment_root` (`root_id`, `id`) USING BTREE,
  INDEX `idx_comment_uid` (`uid`) USING BTREE,
  INDEX `idx_comment_pid` (`pid`) USING BTREE,
  CONSTRAINT `fk_comment_parent` FOREIGN KEY (`pid`) REFERENCES `comment` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='评论（盖楼无限嵌套：root_id 标记楼，pid 直接父+自关联外键）';
```

- `Pid` 用 `sql.NullInt64`（一级评论 NULL），`ParentComment` 自关联外键 `ON DELETE CASCADE`（对齐原设计）。**删除走软删**（UPDATE deleted_at）不触发 CASCADE；级联由 DAO 显式实现——删一级评论整楼软删（`root_id` 命中），删楼内回复仅自身、子回复保留。
- DAO struct 同步 `TableName()` + `autoCreateTime/UpdateTime:milli` + `softDelete:milli`；DDL 同步 `comment/scripts/comment.sql`。

### 1.2 核心查询

| 场景 | SQL 思路 | 命中索引 |
|------|---------|---------|
| 一级评论分页（最新） | `WHERE biz=? AND biz_id=? AND root_id=0 ORDER BY id DESC` offset 分页 | `idx_comment_biz_root` |
| 一级评论分页（最热） | **不在 comment 排序**——见 §3「最热聚合」，core 调 interaction 计数后内存排序 | — |
| 按 id 批量取 | `WHERE id IN (?)`（`BatchGetComments`，供 core 聚合回查） | 主键 |
| 楼内回复（懒加载） | `WHERE root_id=? ORDER BY id ASC LIMIT n OFFSET m` | `idx_comment_root` |
| 评论总数 | `COUNT WHERE biz=? AND biz_id=?` | `idx_comment_biz_root` |

### 1.3 缓存（Cache-Aside，key 定义在 `comment/consts/cache.go`）

| key pattern | 内容 | TTL |
|-------------|------|-----|
| `comment:cnt:{biz}:{bizId}` | 评论总数 | 10min + jitter(0~5min) |

写操作（发表/删除）后清 cnt key。**列表/最热结果缓存放 core 聚合层**（comment 服务只缓存自身能定的计数）。

### 1.4 前端 UI 状态枚举（5 态）

`idle / loading（骨架屏）/ success（列表）/ empty（"还没有评论"）/ error（重试）`；输入态 `editing / submitting / sensitive-rejected`。

## 2. 接口设计

### 2.1 gRPC 契约 `api/proto/comment/v1/comment.proto`（点赞不在此）

```proto
service CommentService {
  rpc CreateComment(CreateCommentRequest) returns (CreateCommentResponse);     // 含敏感词校验
  rpc ListComments(ListCommentsRequest) returns (ListCommentsResponse);        // 一级评论按时间分页+每条前 N 回复
  rpc BatchGetComments(BatchGetCommentsRequest) returns (BatchGetCommentsResponse); // 按 id 批量（core 聚合回查）
  rpc GetReplies(GetRepliesRequest) returns (GetRepliesResponse);              // 楼内回复懒加载
  rpc DeleteComment(DeleteCommentRequest) returns (DeleteCommentResponse);     // 软删+鉴权(仅本人)
  rpc CountComment(CountCommentRequest) returns (CountCommentResponse);
}
// message Comment 含 id/biz/biz_id/commentator/content/root_id/pid/reply_cnt/created_at/children
// 不含 like_cnt/liked —— 由 core 聚合 interaction(biz="comment") 后填入对外 VO
```

放 `webook/api/proto/comment/v1/`，生成 `webook/api/gen/comment/v1/`（对齐 article/interaction）。

### 2.2 HTTP 契约（core 暴露给前端，统一 `{code,msg,data}` + `x-access-token`）

| Method · Path | Auth | Request | Success data | Errors |
|---|---|---|---|---|
| POST `/api/comment/list` | none | `{articleId, sort:"hot"｜"new", offset?, limit}` | `PageResult<Comment>`（含 likeCnt/liked，core 聚合） | 400 |
| POST `/api/comment/replies` | none | `{rootId, offset?, limit}` | `PageResult<Comment>` | 400 |
| POST `/api/comment/create` | Bearer | `{articleId, content, pid?}` | `{comment}` | 401·400·422 敏感词 |
| POST `/api/comment/delete` | Bearer | `{id}` | `{}` | 401·403·404 |
| POST `/api/comment/like` | Bearer | `{commentId, liked}` | `{}` | 401 |

- 评论点赞**不新增接口**，直接走 core 已有的 interaction 点赞 endpoint（`biz="comment"`）
- biz 固定 `"article"`（core 注入，前端只传 articleId）；core handler 遵循 webook Web 层规范（`ginx.WrapReq/WrapClaims` + `*errs.Error`）

## 3. 关键技术决策

| 决策 | 选择 | 理由 |
|------|------|------|
| **评论点赞落点** | **复用 interaction 服务**（`biz="comment"`），comment 不自建、不存点赞 | 点赞是通用互动能力，interaction 已有 (biz+bizId) 点赞/计数基础设施；自建 `comment_like` 是重复造轮子。点赞明细/计数/liked 全在 interaction |
| **最热聚合** | core 内部聚合：`comment.ListComments` 拿该文章一级评论一批 → core 调 **interaction service（同进程）批量取 like_count** → 内存排序 top N | comment 不存 like_cnt 就无法本库排序；core 是 interaction 宿主 + comment client，聚合天然。P0 最热取首屏 top N（评论量可控）；超大文章深分页留 P1 |
| **无限嵌套存储** | `root_id`（标记楼，一级=0）+`pid`（直接父，自关联 FK） | 一索引查整楼（`root_id`）、一索引查一级（`root_id=0`）；无需递归 CTE。可视缩进 ≤3 层是前端逻辑 |
| **敏感词** | 本地 **DFA 字典树** `pkg/sensitive`（词库加载，热更挂 `ConfigChangeCallbacks`） | 无外部依赖、O(n) 匹配。已实现 ✅ |
| **删除** | **整楼级联软删**：删一级评论 → 自身 + 全部 `root_id` 命中回复一条 UPDATE 软删；删楼内回复 → 仅自身软删 + 楼根 `reply_cnt`−1，子回复保留。前端删除确认框对有回复的一级评论提示「N 条回复将一并删除」 | 删除前置提示优于删后留占位尸体。曾用「占位双模式」（`deleted` 标记列保留空壳行），已移除——占位行永久挤占讨论串且需额外标记列/Count 过滤 |
| **点赞接口** | 不复用 `/interaction/like`（其硬编码 biz="article"、收 `{articleId}`），core 另开 **`/comment/like {commentId,liked}`** 内部调 interaction `Like/CancelLike(biz="comment")` | 复用 interaction **数据层**而非 endpoint，不动 article 点赞契约；落点仍在 interaction（biz="comment"） |
| **回复展示** | 后端 `GetReplies` 返回整楼**扁平**回复（带 `pid`）；**P0 前端扁平单层展示**（树形递归缩进 + @提及降级 P1） | 扁平实现简单、移动端不溢出；`pid` 已存，P1 可前端重建树 |

## 4. 前瞻设计（适度——UGC 业务核心，P0 不涉及钱/不可逆）

- **性能**：一级评论 offset 分页（P0），深翻页 P1 改 id 游标；`idx_comment_biz_root` 覆盖过滤+排序；楼内回复 `root_id` 索引
- **并发**：`reply_cnt` 用 `gorm.Expr("reply_cnt+1")` 自增避免读改写竞态；点赞并发由 interaction 负责
- **缓存**：cnt 的 TTL 加 jitter 防雪崩
- **限流**：发表/回复接 `pkg/ratelimit`，防刷评论
- **可观测**：复用 `grpcx` metrics/tracing/errconv 拦截器，metric 命名 `webook_grpc_*`/`webook_db_*`（service 靠 job label 区分）

## 5. 风险清单

| 类别 | 风险 | 缓解 |
|------|------|------|
| 性能 | 最热聚合：core 拿一批评论 + interaction 批量计数 + 内存排序，超大文章评论数压力 | P0 限首屏 top N；P1 interaction 增「父范围热门」或物化视图 |
| 性能 | 单楼回复过多 | 楼内回复独立分页（GetReplies offset） |
| 安全 | 敏感词漏判 / XSS | DFA 词库热更；content 前端转义、后端不存 HTML |
| 回归 | core 新增 comment client 影响启动 | comment 挂掉时 core 评论接口降级返空，不拖垮文章详情主流程 |
| 误删 | 整楼级联删除误操作波及全部回复 | 删除前置确认提示（含子回复数）；软删可人工恢复 |

## 6. 任务拆分（含已完成）

1. ✅ **proto**：`comment.proto`（无 LikeComment，含 BatchGetComments）+ `make gen`
2. ✅ **dao model**：`comment` model（Pid nullable + ParentComment FK，无 like_cnt）+ `init_table` + `scripts/comment.sql`
3. ✅ **dao 查询**：`Insert`(算 root_id+reply_cnt) / `FindById` / `BatchGet` / `PageRoots`(时间) / `ListReplies` / `Delete`(鉴权软删) / `Count`，集成测试全绿
4. **repository**（进行中）：`toDomain`/`toEntity`（单条，批量 `slicex.Map`）+ cnt 的 Cache-Aside
5. **cache**：`RedisCommentCache`（cnt key + jitter）
6. ✅ **sensitive**：`pkg/sensitive` DFA
7. **service**：`CommentService`（创建[敏感词校验+限流]、列表、回复、删除[鉴权]、计数）——无点赞
8. **grpc server**：`comment/grpc/comment.go` 实现 `CommentServiceServer`
9. **ioc + wire**：`InitGRPCServer` 注册 + `wire.go`
10. **core 接入**：comment gRPC client 并入 `internal/ioc/grpc.go`（与 server 共享 `InitGRPCMetrics`）+ `internal/web/comment.go`(handler，list/replies/create/delete/like，聚合 interaction 的 likeCnt/liked + 最热内存排序) + wire
11. **配置/监控**：core wire/ioc；`deploy/prometheus` 加 comment job；comment 5 份 yaml 核对
12. **前端**：`api/comment.ts` + `types/comment.ts` + `CommentSection`/`CommentItem`/`CommentEditor` + `read.tsx` 挂载（点赞调 interaction api）

> 已完成 1/2/3/6；下一步任务 4 repository。
