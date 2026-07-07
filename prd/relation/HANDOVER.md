# relation 模块 — 实现进度 Handover

> 用途：会话中断后据此续接。产品/原型见 `PRD.md`，架构见 `ARCHITECTURE.md`。
> 更新时间锚点：本轮会话（2026-07-06）。

## 已完成（全部 GREEN，可独立验证）

后端 `webook/relation/`（独立 gRPC 微服务，镜像 `interaction/`）：

| 层 | 文件 | 状态 |
|----|------|------|
| domain | `domain/relation.go`（RelationStats/FollowEdge/BlockEdge[含 Id]/RelationState+IsMutual） | ✅ |
| DAO | `repository/dao/relation.go`（3 表 model + 全方法）+ `init_table.go` | ✅ |
| cache | `repository/cache/relation.go`（Redis Hash stats，cache-aside） + `consts/cache.go` | ✅ |
| repository | `repository/relation.go`（cache-aside + 写后失效 + `toDomain<Type>` 方法转换） | ✅ |
| service | `service/relation.go`（自关注/自拉黑校验、Follow 双向拉黑门控、GetRelation 组装） | ✅ |
| errs | `errs/error.go`（ErrFollowSelf/ErrBlockSelf/ErrBlockedTarget/ErrBlockedByTarget/ErrInvalidArg） | ✅ |
| proto | `api/proto/relation/v1/relation.proto` → `make gen` → `api/gen/relation/v1/` | ✅ |
| gRPC server | `grpc/relation.go`（RelationServer + `toPb<Type>`）+ `grpc/validate.go` | ✅ |
| DDL | `scripts/relation.sql`（3 表） | ✅ |
| test | `integration/`（dao/repo/service 三 suite，**32 用例真库全绿**） | ✅ |

**验证命令**（需本机 MySQL:3306 + Redis:6379，库 `webook_test`）：
```bash
cd webook && go test ./relation/integration/...   # ok, 32 tests
```

## 关键决策（已定，勿推翻）

- **独立 gRPC 服务**（非塞 core）：feed/chat 跨服务消费 → 放 core 会反向依赖网关。端口 **8060(HTTP)/8061(gRPC)**（见 CLAUDE.md 端口分配铁律）。
- **关注边 status 翻转**（非软删）：行锁读旧态 + 真翻转才增减 + GREATEST(0) 防负（镜像 interaction.UpsertLike）。
- **计数缓存 cache-aside + 写后失效**（非增量 HINCRBY）：Follow/Unfollow/Block 在 changed 时清双方缓存；Unblock 不改计数不清。
- **事件在 core 生产、不在 relation**：relation 纯同步（对齐 interaction 边界铁律）。core 调 gRPC 写成功且 `changed=true` 才产 `relation_events`（topic+JSON）。
- **拉黑级联**：Block 事务内解除双向关注 + 计数；取消拉黑不恢复关注。
- **昵称/头像聚合在 core web 层** `userSvc.FindByIds`（relation 只存/回 uid）。

## 剩余步骤（按序）

1. ✅ **ioc + wire + main 已完成**：`ioc/{config,db,redis,grpc,etcd,logger,otel,time}.go` + `wire.go`/`wire_gen.go`（`wire ./relation/...` 已生成）+ `main.go`（:8060 HTTP metrics/health + gRPC :8061）。`go build ./...` 通过，服务可独立跑。**仅剩 config 4 份 yaml**（local/dev/staging/prod；test.yaml 已建）→ 归入下方基建。
2. ✅ **core 接入已完成**（`go build ./...` 通过、`goimports -l` 稳定、`go vet` 干净）：
   - `internal/ioc/grpc.go`：`RelationConn`/`InitRelationConn`/`InitRelationClient`（共享 `InitGRPCMetrics`）
   - `internal/web/relation.go`：8 endpoint（follow/unfollow/followees/followers/stat/block/unblock/blocklist），`userSvc.FindByIds` 聚合 name/bio（user 表无头像，前端首字母），`GetRelation` 填关系态；写成功且 `changed` → 产 `relation_events`。路由 `Group("/relation")`（无 /api）；followees/followers/stat 走 OptionalPaths。游标列表返 `{list, nextCursor}`（非 offset PageResult）。
   - `internal/events/relation/{event.go,producer.go}`：`TopicRelationEvents` + `SaramaRelationEventProducer`（key=follower_id）。
   - wire：`internal/wire.go`（kafkaProviderSet 加生产者 + client/handler）+ `internal/integration/setup/wire.go`（`provideTestRelationHandler` 内联 nil client/producer——避 goimports 对 gen/relation/v1 bare import 的重复别名坑）均已 `wire ./internal/...` 重生成。
   - ⬜ 仅剩：前端若直连需 `webook-fe/next.config.ts` 加 `/api/relation` rewrite（走 core /api 则 core 已在 `/relation`，nginx 反代同 comment 覆盖）。
3. ✅ **前端已完成**（`npm run build` + `npm run lint` 绿）：
   - `types/relation.ts` + `api/relation.ts`（8 端点）；`hooks/useCursorList.ts`（游标分页通用）；`utils/format.ts`（formatCount/relativeTime/joinedFor）
   - `components/relation/`：`FollowButton`（4 态，精确对齐 relation.pen）· `UserCard` · `FollowList`
   - `views/user/`：`profile.tsx`（+关注/粉丝 Tab，URL ?tab）· `blocklist.tsx`（+拉黑弹窗在 detail）· `detail.tsx`（他人主页）
   - 路由：`app/(main)/user/profile`(Suspense)、`app/(main)/user/settings/blocklist`、**`app/(auth)/user/[id]`**（公开组 + PublicHeader，PRD「公开·操作需登录」）
   - **他人主页需的后端补齐**（本轮已做）：公开 `POST /user/info` + `POST /article/reader/author`（按作者已发布文章 + likeCnt + likedTotal 获赞聚合）+ reader dao/repo/service 三层；均入 IgnoredPaths；mocks 重生成；build/vet/goimports/unit test 绿
   - ⬜ 遗留小项（非阻断）：获赞 denormalize 到计数器；私信按钮暂跳 `/chat`（P1）；文章卡示点赞+浏览（评论数属 comment 服务未二聚）；移动端响应式类适配、未运行态目检

> **relation 模块至此前后端 + 部署 + 前端全链路完成。** 端到端验收需起 core+relation gRPC+MySQL+Redis+etcd 后手测各页。
4. ✅ **新服务基建已完成**（CLAUDE.md「服务拆分」清单全过）：config 5 份 + `relation/CLAUDE.md`（原有）；本轮补齐 deploy：
   - prometheus job `webook-relation`（target `webook-relation:8060`）· grafana 告警 `deploy/grafana/provisioning/alerting/webook-relation.yml`（up/grpc-error-rate/grpc-p99/goroutines）
   - `deploy/docker-compose.yaml` 服务块 + healthcheck(:8060/health) + depends mysql/redis/etcd；`docker-compose.local.yaml` build override（context=webook/）
   - 8 份 `deploy/.env.*{,.example}` 加 `RELATION_IMAGE_TAG`/`RELATION_APP_ENV`
   - CI `.github/workflows/webook-relation-ci.yml` + 6 兄弟 CI 的 `paths-ignore` 各加 `webook/relation/**`
   - `webook/relation/Dockerfile`（多阶段，context=webook/ 仓根）
   - **无需改**（已核实）：`deploy.sh`（服务名透传）· grafana 看板（`$service`/`label_values` 自动纳入）· prometheus 录制规则（无 cron/lock）· nginx（内部 gRPC，经 etcd 不直连前端）
   - ⚠ 本机无 docker，`docker compose config` 未跑；YAML 已全部 lint 通过、结构镜像 interaction。部署者上线前可跑 `cd deploy && docker compose --env-file .env.dev -f docker-compose.yaml config` 复核。

## 已顺带修复（防再犯，已写规则）

- worker `event/` → `events/`（对齐 core `internal/events` 复数）
- CLAUDE.md：端口分配铁律 + 转换命名多类型情形（`toDomain<Type>`）
- 记忆：Pencil padding `[纵,横]` 踩坑（`pencil-pen-conventions`）
