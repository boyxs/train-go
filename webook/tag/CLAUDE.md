# webook-tag（通用标签服务）

标签本体 + 打标签关联 + 关注订阅的独立 gRPC 微服务。与 `interaction/`、`comment/`、`relation/` 同构平铺布局（不套 `internal/`）。端口 `8070`（HTTP）/`8071`（gRPC）。

## 为什么拆

标签是**零业务耦合的通用能力**：文章、（预留的）用户/对话都可被打标签，core / 未来服务都要平等消费。留在 core：① 打标签/查标签与 core 发布链路耦合、无法独立扩容；② 通用标签模型塞进 core domain 会污染业务边界。抽成独立 gRPC 服务后，core BFF 作 client 经 etcd 调用——与 interaction/relation 同构。

## 边界铁律

- **纯同步 gRPC**：HTTP `:8070` 只暴露 `/metrics` + `/health`，业务全走 gRPC `:8071`。
- **内部不碰 Kafka / cron**：只做同步读写。
- 与 relation/interaction/comment **同库不分库**（webook 库）；表 `tag`（本体）/ `tagging`（多态关联）/ `tag_follow`（关注边）。DDL 真相源 `tag/scripts/tag.sql`（AutoMigrate 之外，与 dao model 严格对齐）。
- **通用性铁律**：`biz` 是入参，方法名/message 名禁硬编码具体业务——用 `BizIdsByTag`（❌ `ArticleIdsByTag`）。insert-or-ensure 一律 `Upsert`（❌ `FindOrCreate`）。

## 接入方

- **core BFF**：`internal/service/tag.go`（`GRPCTagService`）持 `tagCli`（本服务）+ `searchCli`（Recommend 走 search kNN）+ reader/interaction，聚合后经 `internal/web/tag.go` 暴露 HTTP。发文/改标签 → `SyncTags`；下架 → `ClearTags`；typeahead `Suggest`；AI 荐标签 `Recommend`（search kNN）；详情 `Detail`；标签下文章 `TagArticles`；关注 `Follow`/`Unfollow`/`FollowStatus`；批量补名 `BatchBySlugs`/`TagsByBiz`。

## 关键决策

- **通用标签模型**：`tag`（本体，`type` 命名空间）+ `tagging`（`biz`+`biz_id`+`source` 多态）+ `tag_follow`（`uid`→`tag_id` 关注边）。零业务耦合。
- **硬删风格**：untag = 物理删（`tagging`，`uk_tagging_dedup` 保活跃行唯一、无软删幽灵冲突）；`ref_count`/`follow_count` 事务内 `GREATEST(0,…)` 防负维护。故三表均无 `deleted_at`。
- **写路径批量化**：`SyncTags` 一次 `INSERT … ON CONFLICT DO NOTHING` + 回查解析全部 tagId；`SyncByBiz` `CreateInBatches`/`DELETE…IN` + ref_count 同向分组 UPDATE，消发文写侧 N+1（见 ARCHITECTURE §F4）。
- **关注订阅**：`tag_follow` status 翻转（FOR UPDATE 读旧态、仅真翻转增减 `follow_count`），返回 `changed` + 翻转后关注数。`FollowStatus` 独立于 `Detail`（viewer 相关，不进可缓存的纯查询）。
- **本周新增**：`tagging.created_at` 近 7 天滚动窗口 `COUNT`（`Detail` 计算 `since=now-7d`），跨 biz 同 `ref_count` 口径。
- **Cache-Aside**：`Detail` 热点读走 Redis（`tag:detail:{slug}`，JSON + jitter TTL 10min，见 ARCHITECTURE §F5）；`Follow`/`Unfollow` 真翻转 + `SyncByBiz` 精确失效（返 affected tagIds）；`isFollowing`/typeahead/list 不缓存。

## 分层

`grpc/`（适配 pb↔domain + `errconv` 拦截器）→ `service/`（校验/归一/去重/上限 + 窗口计算）→ `repository/`（Cache-Aside，协调 tag/tagging/tag_follow 三 DAO + toDomain/toEntity）→ `dao/` + `cache/`。构造函数返回接口；时间全 `int64` 毫秒；DAO 不依赖 domain。

## 部署

`config/{local,dev,staging,prod,test}.yaml` 5 份同构 + 差异点（otel.env / sample_ratio / server.addr `:8070`/`:8071`）；`data.mysql` + `data.redis`（详情缓存，local/test 明文 6379、dev/staging/prod `${REDIS_PASS}`）。内部服务，**不入 nginx**（gRPC 经 etcd 发现）。基建已落地：

- prometheus job `webook-tag`（target `webook-tag:8070`）+ grafana 告警 `deploy/grafana/provisioning/alerting/webook-tag.yml`
- docker-compose 服务块 + healthcheck(`:8070/health`) + depends mysql/redis/etcd；`docker-compose.local.yaml` build override（context=`webook/`）
- `.env.{local,dev,staging,prod}{,.example}` 的 `TAG_IMAGE_TAG` / `TAG_APP_ENV`
- CI `.github/workflows/webook-tag-ci.yml`（paths 与兄弟服务互斥）+ 兄弟 CI 的 `paths-ignore` 已加 `webook/tag/**`
- `tag/Dockerfile`（多阶段，context=`webook/` 仓根）

无需改动（服务无关 / 内部服务）：`deploy.sh`（服务名透传）· grafana 看板（`$service` 变量自动纳入）· nginx（gRPC 经 etcd，不直连前端）。
