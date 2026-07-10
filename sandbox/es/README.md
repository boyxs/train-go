# sandbox/es — go-elasticsearch/v9 全面用法示范

用官方 [`go-elasticsearch/v9`](https://github.com/elastic/go-elasticsearch) 的 **TypedClient**，对齐真实 **ES 9.x**，把索引管理 / 文档 CRUD / 批量 / 搜索 / 聚合 / 计数 / 高级检索的常见用法**一次讲全**。

- **交付形态**：由**集成测试驱动**的薄封装 `DocStore`，**无 `main`** —— 每个方法演示一类 ES 操作，每个测试连**真实 ES** 断言其行为。跑 `go test` 就是跑整套示范。
- **风格对齐**：查询/映射沿用项目 `webook/internal/repository/dao/article_search.go` 的 `map[string]any` + `Raw(bytes.Reader)` 写法，直观呈现 ES DSL；响应统一走 TypedClient 的强类型结构（`resp.Hits` / `resp.Aggregations`）。

> 独立 module（`module es`），不进 webook 的 go.work，对现有代码零侵入。

## 前置条件

- 一个可达的 **ES 9.x**（本 demo 在 **9.3.2** 上验证）。
- 默认连 `http://127.0.0.1:9200`，凭据默认 `elastic` / `elastic`。可用 `ES_ADDR` / `ES_USER` / `ES_PASS` 覆盖。

```bash
# 起一个本地单机 ES（开 xpack.security，Basic auth over HTTP 免 TLS，密码 elastic）
docker run -d --name es -p 9200:9200 \
  -e discovery.type=single-node \
  -e xpack.security.enabled=true \
  -e xpack.security.http.ssl.enabled=false \
  -e ELASTIC_PASSWORD=elastic \
  elasticsearch:9.3.2
```

## 运行

```bash
cd sandbox/es
go test ./... -v                                     # 连 127.0.0.1:9200，elastic/elastic
ES_ADDR=http://host:9200 ES_PASS=xxx go test ./...   # 覆盖地址/凭据
```

每个测试用**各自独占**的索引（名字由测试名生成，`es_demo_<test>`），跑前建、跑后删，互不干扰、并行安全，不碰你其它数据。

## 目录结构

按「能力」分文件，多数源文件与其测试一一对应（`xxx.go` ↔ `xxx_test.go`）；`doc.go`/`store.go` 的实体与共享脚手架经其它能力测试间接覆盖，不单开测试文件：

| 文件 | 职责 |
|------|------|
| `doc.go` | 文档实体 `Doc` + 索引 `mapping` + `_source` 解析 |
| `store.go` | `DocStore` + 客户端工厂 `NewClient` + 共享结果类型 + 错误判定（`IsConflict`/`IsNotFound`） |
| `index.go` | **A 索引管理**：建 / exists / 读 mapping / 删（幂等） |
| `document.go` | **B 文档 CRUD**：index / create / get / update / delete / exists |
| `bulk.go` | **C 批量 Bulk**：批量写 / 混合操作 / 部分失败逐条统计 |
| `search.go` | **D 搜索 + F 计数**：通用 `Search(body)` / `Count(body)` |
| `aggregation.go` | **E 聚合**：`TermsCount` / `Stats`（强类型 union 解析封装） |
| `advanced.go` | **高级**：PIT + search_after 深分页、scroll 遍历、mget 批量取 |
| `helper_test.go` | 共享测试脚手架：客户端、freshStore / seedStore、样本数据 |

## 数据模型

`Doc` 字段刻意覆盖 ES 主要类型：

| 字段 | Go 类型 | ES type | 演示 |
|------|--------|---------|------|
| `Id` | `int64` | `long` | 文档主键 / `_id` |
| `Title` | `string` | `text` | 全文分词（match / multi_match / 高亮） |
| `Category` | `string` | `keyword` | 精确匹配（term）/ 分组聚合 / collapse |
| `Tags` | `[]string` | `keyword` | 多值精确匹配（terms） |
| `Score` | `float64` | `double` | 范围 / 排序 / 指标聚合 / 脚本打分 |
| `Views` | `int` | `integer` | 数值字段（function_score） |
| `Content` | `string` | `text` | 全文分词 + 高亮 |
| `CreatedAt` | `int64` | `date`(`epoch_millis`) | 时间范围/排序（遵循项目时间铁律：Unix 毫秒 `int64`） |

## 能力清单

### A 索引管理（`index.go`）
| 方法 | 说明 |
|------|------|
| `CreateIndex` | 用显式 mapping + settings 建索引 |
| `IndexExists` | 判断索引是否存在 |
| `GetMapping` | 读回 mapping 原始 JSON（`Perform` 拿原始响应体） |
| `DeleteIndex` | 删索引；不存在（404）视为已删，幂等 |

### B 文档 CRUD（`document.go`）
| 方法 | 说明 |
|------|------|
| `Index` | 按 `Id` upsert（存在即覆盖） |
| `Create` | 严格新建；已存在 → **409**（`IsConflict` 判定） |
| `Get` | 按 `Id` 取；不存在返回 `(_, false, nil)` |
| `Update` | 部分字段更新；文档不存在 → **404**（`IsNotFound` 判定） |
| `Delete` | 删除，返回是否确实删了 |
| `DocExists` | 判断文档存在 |

### C 批量 Bulk（`bulk.go`）
| 方法 | 说明 |
|------|------|
| `BulkIndex` | 批量写入一组文档 |
| `Bulk` | 混合 index/create/update/delete；手拼 NDJSON，逐条读 `items` 统计成功/失败（**部分失败不整体报错** —— bulk 语义），失败项带状态码 + 原因 |

### D 搜索（`search.go`，通用 `Search(body map[string]any)`）
`match_all` · `match`(分词) · `term`(精确) · `terms`(多值) · `bool`(must/filter/must_not) · `range`(数值/时间) · 分页(from/size) · 排序(sort) · 高亮(highlight) · `_source` 字段过滤 · 空结果。

### E 聚合（`aggregation.go`）
| 方法 / 用法 | 说明 |
|------|------|
| `TermsCount(field)` | terms 分组计数 → `key→doc_count` |
| `Stats(field)` | stats 指标：min/max/avg/sum/count |
| 嵌套聚合 | 直接读 `SearchResult.Aggs`（terms 组内再 avg），见 `aggregation_test.go` |

> 关键：`Search` 内部对搜索请求设了 `TypedKeys(true)`，聚合响应才带类型前缀（`sterms#name`），从而解析成强类型 `types.Aggregate` union。

### F 计数（`search.go`）
`Count(nil)` 全部；`Count(body)` 带 query。

### 高级（`advanced.go` + `advanced_test.go`）
| 能力 | 方法 / DSL | 说明 |
|------|-----------|------|
| **深分页** | `OpenPIT` / `SearchAfter` / `ClosePIT` | Point-in-Time 快照 + `search_after` 游标翻页，跨页数据视图一致 |
| **遍历** | `ScrollAll` | scroll 滚动遍历全量（导出场景；新版已被 search_after+PIT 取代，此处演示经典用法） |
| **批量取** | `MGet(ids)` | 一次 RTT 按多个 `_id` 取回 |
| **模糊查询** | `Search` DSL | `fuzzy`（编辑距离）/ `wildcard`（通配符）/ `prefix`（前缀）/ `multi_match`（多字段） |
| **脚本打分** | `Search` DSL | `function_score`（field_value_factor）/ `script_score`（项目 kNN 同款模式） |
| **折叠** | `Search` DSL | `collapse` 按字段折叠去重 |

## 关键设计决策

1. **客户端选 v9**：客户端 major 必须匹配 server major（ES 9.x → `go-elasticsearch/v9`）。项目 webook 也已从 v8 同步升到 v9。
2. **查询用 `map[string]any` + `Raw`**：对齐项目现有写法，直观暴露 ES 查询 DSL（学 ES 本身），比强类型 DSL builder 更易读；响应仍走强类型解析拿类型安全。
3. **每个测试独占索引**：名字由 `t.Name()` 生成，避免所有测试共享一个索引名反复删建导致的偶发竞态（索引重建有传播窗口，bulk 写会偶发 `index_not_found`）。
4. **时间统一 `int64` 毫秒**：遵循项目时间铁律，mapping 用 `date`/`epoch_millis`。
5. **断言库用 `testify`**：项目统一（test-only 依赖）。

## 注意事项

- **`Refresh(true)`**：demo/测试里所有写操作都刷新以便立即可搜；**生产环境勿这么用**（每次 refresh 很伤性能，靠默认 1s 周期刷新即可）。
- **单分片零副本**：mapping settings `number_of_shards:1 / number_of_replicas:0`，仅为单机 demo 立即可用；生产按集群规模设。
- **scroll 已过时**：优先用 `search_after` + PIT 做深分页/遍历。

## 参考

- 项目内 ES 实战：`webook/internal/repository/dao/article_search.go`（BM25 + kNN 混合搜索）、`webook/internal/ioc/es.go`（TypedClient 初始化 + 建索引）、`webook/migrator/pipeline/{source,sink}/es.go`（低层 `Client` + `esapi` 做迁移源/汇）。
