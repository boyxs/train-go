# 用户关系架构设计

> 配套需求见同目录 `PRD.md`，原型见 `prototypes/*.png`。
> 服务：webook 用户关系微服务（`webook/relation/`，gRPC server）+ webook-core（HTTP 网关 + relation gRPC client + user 昵称聚合）+ 前端。
> 关键约束：**关注流 feed 后续单开独立服务**——relation 只暴露关系查询 gRPC + `relation_events` 领域事件（topic+JSON，两端各定义），本期不内嵌 feed、不反向依赖 feed。

## 0. 调用拓扑（对齐 interaction / comment，非臆测）

```
前端 → nginx → core HTTP /api/relation/*  ──(gRPC, etcd 服务发现)──▶  webook-relation (gRPC server)
                     │                                                      │
              鉴权/VO 转换/昵称聚合                                service → repository(cache-aside) → dao + cache
                     │                                                      │
              user service（FindByIds 填昵称/头像）· gRPC 写成功且 changed ─▶ core 生产 Kafka: relation_events（relation 服务纯同步，不碰 Kafka，对齐 interaction 边界铁律）
                                                                            │
                                              （未来）webook-feed 订阅做写/读扩散 · webook-worker 订阅做「被关注」通知(P1)
                                                                            │
                                      chat 私信前 gRPC 调 relation.GetRelation 校验拉黑（P1）
```

**为什么独立服务而非放 core**：relation 是高频写 + 通用计数，与 `interaction` 完全同构；且 **feed（未来服务）、chat 都要跨服务消费关系数据**——放 core 会让后端服务反向依赖网关（循环/耦合）。独立 gRPC 服务让 core/feed/chat 都作为 client 平等调用，与 interaction 被 core/comment 消费的模式一致。→ 触发 CLAUDE.md「服务拆分」14 项清单（见 §6.2）。

- **core 侧新增**：relation gRPC client 并入 `internal/ioc/grpc.go`（`RelationConn`/`InitRelationClient`，共享 `InitGRPCMetrics` 单例）+ `internal/web/relation.go`（HTTP handler，聚合 user 昵称/头像 + viewer 关系态）+ wire
- **relation 侧填充**：proto / dao / repository / cache / events producer / service / grpc server / ioc.InitGRPCServer + wire（拷 `interaction/` 骨架）

## 1. 数据设计

### 1.1 表结构（3 张，`relation` 库）

关注边用 **status 翻转**（非软删）——镜像 `interaction.UserInteraction` 的「行锁读旧值 + 真翻转才增减 + GREATEST(0,…)」，天然幂等、计数一致、避开软删唯一索引坑。

```sql
-- 关注边：单向 follower → followee
CREATE TABLE `relation_follow` (
  `id`          bigint  NOT NULL AUTO_INCREMENT COMMENT '主键',
  `follower_id` bigint  NOT NULL DEFAULT 0  COMMENT '关注者 uid',
  `followee_id` bigint  NOT NULL DEFAULT 0  COMMENT '被关注者 uid',
  `status`      tinyint NOT NULL DEFAULT 0  COMMENT '1=关注中 0=已取关（翻转，不物理删）',
  `created_at`  bigint  NOT NULL DEFAULT 0  COMMENT '首次关注时间（Unix 毫秒）',
  `updated_at`  bigint  NOT NULL DEFAULT 0  COMMENT '最近状态变更（Unix 毫秒）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_relation_follow_edge` (`follower_id`, `followee_id`),
  KEY `idx_relation_follow_er` (`follower_id`, `status`, `id`),
  KEY `idx_relation_follow_ee` (`followee_id`, `status`, `id`)
) ENGINE=InnoDB CHARSET=utf8mb4 COMMENT='用户关注边（单向；status 翻转维护）';

-- 聚合计数：每用户一行（无删除路径，故不加 deleted_at，同 interaction 计数表）
CREATE TABLE `relation_stats` (
  `id`           bigint NOT NULL AUTO_INCREMENT,
  `uid`          bigint NOT NULL DEFAULT 0 COMMENT '用户 uid',
  `followee_cnt` bigint NOT NULL DEFAULT 0 COMMENT '关注数（该用户关注了多少人）',
  `follower_cnt` bigint NOT NULL DEFAULT 0 COMMENT '粉丝数（多少人关注该用户）',
  `created_at`   bigint NOT NULL DEFAULT 0,
  `updated_at`   bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_relation_stats_uid` (`uid`)
) ENGINE=InnoDB CHARSET=utf8mb4 COMMENT='用户关系聚合计数';

-- 黑名单：uid 拉黑 blocked_uid（低频，取消=物理删）
CREATE TABLE `relation_block` (
  `id`          bigint NOT NULL AUTO_INCREMENT,
  `uid`         bigint NOT NULL DEFAULT 0 COMMENT '拉黑发起者',
  `blocked_uid` bigint NOT NULL DEFAULT 0 COMMENT '被拉黑者',
  `created_at`  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_relation_block_edge` (`uid`, `blocked_uid`),
  KEY `idx_relation_block_uid` (`uid`, `id`)
) ENGINE=InnoDB CHARSET=utf8mb4 COMMENT='用户拉黑黑名单';
```

DAO model 用 `autoCreateTime:milli`/`autoUpdateTime:milli`；`relation_follow`/`relation_block` 无 `soft_delete`（用 status/物理删）。DDL 同步 `relation/scripts/relation.sql`。

### 1.2 核心查询

| 场景 | SQL 思路 | 命中索引 |
|------|---------|---------|
| 我的关注列表（游标） | `WHERE follower_id=? AND status=1 AND id<?cursor ORDER BY id DESC LIMIT n` | `idx_relation_follow_er` |
| 我的粉丝列表（游标） | `WHERE followee_id=? AND status=1 AND id<?cursor ORDER BY id DESC LIMIT n` | `idx_relation_follow_ee` |
| feed 写扩散拉全部粉丝 | 同上按 id 游标分批扫 | `idx_relation_follow_ee` |
| isFollowing(A,B) / 批量关系态 | `WHERE follower_id=? AND followee_id IN(?) AND status=1` | `uk_relation_follow_edge` |
| 计数 | 读 `relation_stats.uid`（走缓存，miss 回源；兜底 `COUNT` 校准） | `uk_relation_stats_uid` |
| 黑名单列表 / 是否拉黑 | `WHERE uid=? [AND blocked_uid=?]` | `uk_relation_block_edge`/`idx_relation_block_uid` |

### 1.3 缓存（Cache-Aside，key 定义在 `relation/consts/cache.go`）

| key pattern | 内容 | TTL |
|-------------|------|-----|
| `relation:stats:{uid}` | Hash `{followee_cnt, follower_cnt}` | 15min + jitter(0~5min) |

- 写路径（follow/unfollow/block 级联）成功后：present-only Lua `HINCRBY`（缓存已存在才增减，镜像 `interaction` 的 `incr_if_present.lua`），避免未预热写脏。
- 读路径：`GetStats` cache-aside，miss 回源 DB + 回填（同步复用 caller ctx，禁 fire-and-forget）；批量列表页计数走 `BatchGetStats` 收集 miss 再批量回源。
- `isFollowing/isMutual/isBlocked` 为唯一索引点查，**P0 不缓存**（走 DB，KISS）；大 V 热点判定留 P1。

### 1.4 前端 UI 状态枚举

- 关注按钮：`unknown(未加载) / not-following（关注）/ following（已关注）/ mutual（互相关注）/ blocked（无法关注,禁用）/ pending（乐观切换中）`
- 列表：`idle / loading（骨架）/ success / empty（"还没有关注/粉丝"）/ error（重试）`；游标 `loading-more / no-more`

## 2. 接口设计

### 2.1 gRPC 契约 `api/proto/relation/v1/relation.proto`

```proto
service RelationService {
  rpc Follow(FollowRequest) returns (FollowResponse);           // 幂等；被拉黑/拉黑校验；成功推 relation_events
  rpc Unfollow(FollowRequest) returns (FollowResponse);         // 幂等
  rpc ListFollowees(ListRequest) returns (ListResponse);        // 我关注的（游标, id DESC）
  rpc ListFollowers(ListRequest) returns (ListResponse);        // 我的粉丝（游标）
  rpc GetStats(GetStatsRequest) returns (RelationStats);        // {followee_cnt, follower_cnt}
  rpc GetRelation(GetRelationRequest) returns (GetRelationResponse); // viewer 对一批 target 的关系态
  rpc Block(BlockRequest) returns (BlockResponse);              // 级联解除双向关注（事务内）
  rpc Unblock(BlockRequest) returns (BlockResponse);
  rpc ListBlocks(ListRequest) returns (ListResponse);
}
// ListResponse: repeated Edge { int64 uid; int64 created_at }  + int64 next_cursor
// GetRelationResponse: map<int64, RelationState>；RelationState { bool is_following; bool is_followed_by; bool is_blocked; bool is_blocked_by }
//   → core 派生 is_mutual = is_following && is_followed_by；chat 私信校验读 is_blocked_by(to 拉黑了 from)
```

放 `webook/api/proto/relation/v1/`，`make gen` 出 `webook/api/gen/relation/v1/`（对齐 interaction/comment）。**gRPC 只回 uid + 关系态，不回昵称/头像**（由 core 聚合）。

### 2.2 HTTP 契约（core 暴露前端，统一 `{code,msg,data}` + `x-access-token`）

| Method · Path | Auth | Request | Success data | Errors → 用户可见消息 |
|---|---|---|---|---|
| POST `/api/relation/follow` | Bearer | `{followeeId}` | `{}` | 401 请先登录 · 400 不能关注自己 · 409 已拉黑/被对方拉黑，无法关注 |
| POST `/api/relation/unfollow` | Bearer | `{followeeId}` | `{}` | 401 |
| POST `/api/relation/followees` | 可选 | `{userId, cursor?, limit≤50}` | `PageResult<UserBrief + isMutual>` + `nextCursor` | 400 |
| POST `/api/relation/followers` | 可选 | `{userId, cursor?, limit≤50}` | `PageResult<UserBrief + isFollowedBack>` + `nextCursor` | 400 |
| POST `/api/relation/stat` | 可选 | `{userId}` | `{followeeCnt, followerCnt, isFollowing, isMutual, isBlocked, isBlockedBy}` | 400 |
| POST `/api/relation/block` | Bearer | `{targetId}` | `{}` | 401 · 400 不能拉黑自己 |
| POST `/api/relation/unblock` | Bearer | `{targetId}` | `{}` | 401 |
| POST `/api/relation/blocklist` | Bearer | `{cursor?, limit≤50}` | `PageResult<UserBrief + blockedAt>` + `nextCursor` | 401 |

- 昵称/头像由 core web 层 `userSvc.FindByIds(uids)` 批量聚合填 `UserBrief`（镜像 comment 的 `resolveNames`，避免 N+1，user 服务失败降级填占位）。
- viewer 登录态：`ginx.MustClaims`（写操作）/ `ginx.Claims` 可选（列表/stat）；未登录时 isFollowing 等返回 false。
- 路由组 `/relation`（无 `/api` 前缀，见 CLAUDE.md 铁律）；`followees/followers/stat` 走 `OptionalPaths`。

### 2.3 领域事件契约（Kafka `relation_events`，两端各定义、不共享代码）

**生产落点在 core，不在 relation 服务**——relation 是纯同步 gRPC 服务（对齐 `interaction/CLAUDE.md` 边界铁律「拆分服务内部不碰 Kafka，异步统一归 worker」）。core 调 `relation.Follow/Unfollow/Block` 成功后，**仅当返回 `changed=true`（真状态翻转）** 才生产事件，避免重复关注刷事件。

```
生产侧 internal/events/relation/event.go：  const TopicRelationEvents = "relation_events"
消费侧 worker/event/relation.go：           同名常量（注释「必须与生产端一致」）+ contract_test 守护漂移
payload RelationEvent { Type("follow"|"unfollow"|"block"|"unblock"); FollowerId; FolloweeId; Ts int64 }
key = follower_id（同一关注者有序，利于未来 feed 写扩散按人聚合）
```

- gRPC 响应带 `changed bool`：`Follow/Unfollow`（是否真翻转）、`Block/Unblock`（是否真变更）；core 据此门控 `Producer.ProduceEvent(ctx, topic, key, json)`。
- **P0 只生产**（定义边界）；消费方——被关注通知(P1, worker)、feed 扩散(未来独立服务)——各自订阅，不改 relation。
- 复用 core 现有 `internal/events` 基础设施（`Producer.ProduceEvent` + `LazyProducer`），与 interaction 事件同款。

## 3. 关键技术决策

| 决策 | 选择 | 理由 |
|------|------|------|
| **服务边界** | 独立 gRPC 服务 `webook/relation/`（拷 interaction 骨架） | 高频写+通用计数，与 interaction 同构；feed/chat 跨服务消费 → 放 core 会反向依赖网关 |
| **关注边存储** | `status` 翻转（1/0）+ 唯一边索引，非软删 | 镜像 interaction.UserInteraction：幂等、计数一致；避开 GORM 软删+唯一索引的 NULL 回填坑 |
| **计数维护** | 事务内行锁读旧 status → 真翻转才 `GREATEST(0, cnt±1)` upsert `relation_stats`；缓存 present-only `HINCRBY` | 防重复关注刷计数、防并发竞态、防负数（照搬 interaction.UpsertLike） |
| **拉黑级联** | Block 事务内：insert `relation_block` + 解除**双向** `relation_follow`（各自 status→0 + 计数−1）+ 推 unfollow 事件 | 拉黑语义要求断开关系；事务原子避免半成品状态 |
| **取消拉黑** | 仅物理删 `relation_block` 行，**不恢复**关注 | 关注是用户主动行为，不应因取消拉黑自动重建 |
| **昵称聚合落点** | core web 层 `userSvc.FindByIds`，relation 只存 uid | 与 comment 一致；relation 不耦合 user 表 |
| **isFollowing/mutual** | 唯一索引点查，P0 不缓存 | KISS；大 V 热点判定 P1 再上缓存 |
| **分页** | id 游标（`id < cursor ORDER BY id DESC`），非 offset | 关系列表可能很大，游标稳定不漏不重（PRD 已定） |
| **feed 解耦** | 只暴露 gRPC 查询 + `relation_events`，不内嵌信息流 | feed 后续独立服务，通过 topic/gRPC 消费，relation 零改动 |

## 4. 前瞻设计（业务核心 · 多方接入：feed/chat/通知消费）

| 维度 | 问题 | 方案 |
|------|------|------|
| 扩展性 | 新消费方接入改动多大？ | 面向能力：gRPC 查询 + `relation_events` topic 为边界；feed/通知只订阅/调用，relation 不改 |
| 可用性 | relation 挂了 core 主流程能跑吗？ | core 调 relation 加超时+熔断（镜像 `KafkaInteractionService` circuitbreaker）；降级：关注态返 unknown、计数返缓存或 0；不拖垮主页/文章详情 |
| 容错性 | 重复/并发/极端输入安全吗？ | follow/unfollow/block 幂等；级联事务原子；`GREATEST(0,…)` 防负；禁自关注/自拉黑 |
| 可观测性 | 5 分钟能定位吗？ | 复用 grpcx metrics/tracing/errconv；metric 命名 `webook_grpc_*`/`webook_db_*`（service 靠 job label 区分，禁 `webook_relation_*`） |

## 5. 风险清单

| 类别 | 风险 | 缓解 |
|------|------|------|
| 性能 | 大 V 百万粉丝列表 / feed 写扩散拉全量粉丝 | 游标分页 + `idx_relation_follow_ee`；扩散分批异步；计数走缓存 |
| 并发 | 同一对 follow/unfollow 并发 → 计数错乱 | 事务行锁读旧 status + 真翻转才增减 + 幂等 |
| 一致性 | 缓存计数与 DB 漂移 | present-only 增量 + TTL 回源校准；异常降级实时 `COUNT` |
| 安全 | 关注/拉黑自己、越权 | 服务端校验 followee≠follower、target≠uid；写操作强制登录 |
| 回归 | core 新增 relation client 影响启动 / 拖垮主流程 | client 懒连接 + 熔断降级；relation 不可用时关系接口返降级值 |
| 契约漂移 | 生产/消费两端 topic/JSON 不一致 | `worker/event/relation_contract_test.go` 守护（同 interaction） |

## 6. 任务拆分

### 6.1 relation 服务 + core 接入 + 前端

| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 1 | `relation.proto` + `make gen` | 无 | 生成 `api/gen/relation/v1` |
| 2 | DAO model（3 表）+ `init_table` + `scripts/relation.sql` | 1 | AutoMigrate 通过 |
| 3 | DAO 查询：Follow/Unfollow（行锁翻转+计数 TX）/ ListFollowees/ListFollowers（游标）/ GetStats / BatchGetRelation / Block(级联 TX)/Unblock/ListBlocks | 2 | dao_test 全绿（幂等/并发/级联用例） |
| 4 | cache：`RedisRelationCache`（stats key + jitter + present-only Lua） | 2 | cache_test |
| 5 | repository：`CacheRelationRepository`（cache-aside + toDomain/toEntity） | 3,4 | repo_test |
| 6 | core 事件：`internal/events/relation/{event.go,producer.go}`（topic 常量 + Produce，复用 core Producer 基础设施） | 无 | 生产可发 |
| 7 | service：`InternalRelationService`（follow/unfollow[校验拉黑]/block[级联]/列表/stat/关系态；**返回 changed，不产事件**） | 5 | service_test |
| 8 | grpc server：`relation/grpc/relation.go` 实现 `RelationServiceServer` | 7 | 起服务通 |
| 9 | ioc + wire：`InitGRPCServer` + `relation/wire.go`（拷 interaction） | 8 | 编译+启动 |
| 10 | core 接入：`internal/ioc/grpc.go` 加 RelationConn/Client（共享 InitGRPCMetrics）+ `internal/web/relation.go`（8 endpoint，聚合 userSvc.FindByIds + 关系态，**写成功且 changed 时经 #6 producer 发事件**）+ wire | 8,6 | HTTP 打通 |
| 11 | worker 消费（P1）：`worker/event/relation.go` + `worker/consumer/relation.go`（被关注通知）+ contract_test | 6 | 消费落库/通知 |
| 12 | 前端：`api/relation.ts` + `types/relation.ts` + 关注按钮组件(3 态) + `/user/[id]` 主页 + profile 关注/粉丝 Tab + `/user/settings/blocklist` + 移动端 | 10 | 原型对齐 |

### 6.2 新服务基建（CLAUDE.md「服务拆分」14 项，缺一视为半成品）

config 5 份 yaml（otel.service_name/sample_ratio/http.addr 差异）· wire · prometheus job=relation · grafana 告警(up/5xx/P99/goroutines) · dashboard · docker-compose(服务+healthcheck+nginx depends) · nginx upstream+`/api/relation` 路由 · deploy.sh · `.env.<env>`(RELATION_ 前缀) · CI workflow(paths 互斥) · Dockerfile(context=webook/) · metric 命名 `webook_<subsystem>_*` · `relation/CLAUDE.md` + CHANGELOG。

> 收尾用 `grep -rn 'relation'` 全仓扫，确认 14 类同步。

---

> **Gate 1：本文档确认后才进 `workflow:tdd` 写代码。** 未确认不写生产代码。
