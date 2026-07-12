# webook-search（内容检索服务）

ES + 向量 embedding 的独立 gRPC 微服务。与 `interaction/`、`comment/`、`relation/` 同构平铺布局（不套 `internal/`）。

## 为什么拆

检索是 **ES + embedding 的重资源能力**，且要独立伸缩、独立部署。留在 core：① core 发布/阅读链路与 ES 索引耦合、无法单独扩容；② chat（RAG / `search_articles` 工具）要跨服务消费检索。抽成独立 gRPC 服务后，core / chat 都作平等 client 经 etcd 调用——与 interaction/relation 同构。

## 核心模型（业界标准，务必先读）

> **一句话：通用的是「基础设施」，不通用的是「相关性」。共享 infra，不共享契约。**

多实体检索（文章 / 用户 / 评论……）**不做「一套归一化 doc + 通用 query」的强 generic**——那是漏抽象。因为能归一的只是存储形状，漏的是**查询语义 / 相关性**，而检索的价值全在后者：

| | article | user（未来） | comment（未来） |
|---|---|---|---|
| 查询方式 | 语义(向量 kNN)+BM25 | 昵称前缀 typeahead | 全文匹配正文 |
| ES 映射 | `dense_vector`+text+keyword | `search_as_you_type`/completion+edge-ngram | text+keyword(article_id) |
| 要 embedding | **要** | **不要** | 可要可不要 |
| facet | tags 聚合 | 基本无 | 基本无 |

把它们硬塞进一个通用管道 → 给 user 硬套向量（白烧 embedding、相关性还错），又给不了它要的前缀补全。**配置能表达「facet 哪个字段」，表达不了「kNN 还是前缀补全」**——那是查询代码。

**这与「comment/tag 能通用、search 不能」是同一根线**：comment/tag 通用是因为**实体统一**（`biz` 只是命名空间列）；search 不能通用是因为**相关性不统一**。

### 落地形态

本服务 = **「检索基础设施之家」**，内放 N 个 per-biz 打字化搜索能力，共用底座：

- **共享（真通用的基础设施）**：ES 集群 + client、embedding client（只给需要的 biz 用）、索引生命周期(ensureIndex/别名)、gRPC/部署/config/监控脚手架。
- **per-biz（天生不同的相关性）**：各自的索引映射 + 查询逻辑 + facet + 要不要向量。打字化、各自调优，**不硬凑 generic**。

当前只有 `article` 一个 biz（`SearchArticles`/`IndexArticle`/`RemoveArticle`/`RecommendTags`）。

## 加一个可搜 biz 的标准配方（user / comment 来了照做）

> 原则「有第二个例子再抽象」：加第二个 biz 时，先照抄下面步骤各写一份，等 2~3 个后再抽公共 query 片段，不提前抽。

1. **proto**：加 typed RPC（`SearchUsers`/`SearchComments`…）+ 各自 typed Req/Resp。**不搞有损的归一 envelope**；若响应能统一成 `{id,score}+facets` 由调用方回源水合，也可，但请求维度按 biz 打字化。
2. **索引**：`<biz>_index.json` 一份**贴该 biz 查询需要的 mapping**（user 用 completion/edge-ngram、不放 dense_vector；comment 放 text+article_id）。别名 `<biz>` → 物理 `<biz>_v1`（见下「别名+版本化」）。
3. **DAO**：新增该 biz 的 DAO（或让 `ElasticXxxDAO` 各管各的索引），查询体按该 biz 写，**复用** ES client。
4. **service**：该 biz 的搜索逻辑；要 embedding 才注入 embedder，不要就别注。
5. **grpc**：`SearchServer`（骨架 `server.go`）实现新 biz 的 RPC，方法 + pb↔domain 映射放 `grpc/<biz>.go`（如 `article.go`，将来 `user.go`）——一个 `SearchServer` 挂全部 biz（proto 单 service 单实现）。
6. **ioc/wire**：注册新 DAO/service；ensureIndex 在启动为新 biz 建索引+别名。
7. **core BFF**：core 加对应入口 + 该 biz 的展示富化（article 补 interaction+tag名；user 补关注态……富化天生 per-biz，不必通用）。

## 索引别名 + 版本化（业界标准，铁律）

app 与查询**只认稳定的逻辑「别名」**（config `data.es.index`，如 `article`）；物理索引是版本化的 `<别名>_v1`。

- `dao.ensureIndex` 幂等确保 `别名 → 物理索引`：别名在→完成；物理不在→按 `article_index.json` 建；再把别名绑到物理（**存量部署只有物理索引、无别名时，本步补挂别名、数据不动**，平滑迁移）。写/查全程走别名（`Upsert/Delete/Search/RecommendTags` 用 `d.index`=别名）。
- **零停机 reindex**（改 mapping 时）：建 `<别名>_v2` → reindex 数据 → **原子切别名** `<别名>` 从 v1→v2 → 删 v1。app 全程无感。
- mapping 单一真相源 = `repository/dao/article_index.json`（Go `//go:embed` + `mk/es.mk create-index` 读同一份，无内联漂移）。手动运维见 `make -f mk/es.mk help`（`ES_INDEX`=别名、`ES_PHYSICAL`=物理）。

## 边界 / 分层

- **纯同步 gRPC**：HTTP `:8080` 只 `/metrics`+`/health`，业务全走 gRPC `:8081`。
- `grpc/`(`SearchServer` 骨架 `server.go` + 各 biz 方法 `article.go`…, pb↔domain) → `service/`(校验/分页/embed 编排、降级) → `repository/`+`dao/`(ES)；`ioc/`(es/embedding/redis/grpc/etcd)。构造函数返回接口，实现带技术前缀（`ElasticArticleDAO`）。
- **embedding** 用 `pkg/embedding`（OpenAI/Ollama/Failover + Redis 缓存 `CachedClient`）；embed 失败降级（不索引/搜索报可感知错误）。
- 时间全 `int64` 毫秒；ES mapping `epoch_millis`。

## 部署

`config/{local,dev,staging,prod,test}.yaml` 5 份同构 + 差异点（otel.env/sample_ratio/server.addr `:8080`/`:8081`、data.es/embedding/ollama/redis）。内部服务，**不入 nginx**（gRPC 经 etcd 发现）。端口段 `808x`（tag `807x`、下一个新服务 `809x`）。metric 统一 `webook_<subsystem>_*`（`webook_es_*`/`webook_grpc_*`），靠 prometheus `job` label 区分。
