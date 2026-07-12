# ============================================================
# ES (Elasticsearch) 管理命令
# 用法: make -f mk/es.mk <target> [ID=xxx] [Q=xxx]
# ============================================================

ES_HOST  := http://localhost:9200
# ES_INDEX 是稳定「别名」（app 与查询都认它）；物理索引是版本化的 $(ES_PHYSICAL)。
# 查询/文档类命令走别名；仅索引生命周期（create/delete/reset）动物理索引。
ES_INDEX    := article
ES_PHYSICAL := $(ES_INDEX)_v1
# mapping 单一真相源 = search 服务的 embed 文件（Go 侧 //go:embed 读同一份），create-index 直接读它、杜绝内联漂移。
ES_MAPPING  := search/repository/dao/article_index.json

# ES 认证（webook-es 开了 xpack.security 后必带）。本地默认 elastic/elastic，
# 覆盖：make -f mk/es.mk status ES_PASS=xxx。连无认证 ES 时带上凭据也无害（服务端忽略）。
ES_USER  ?= elastic
ES_PASS  ?= elastic
ES_AUTH  := -u $(ES_USER):$(ES_PASS)

STEP := [>]
INFO := [-]
WARN := [!]

# 可覆盖参数
ID  ?= 1
Q   ?= Go并发

.PHONY: help \
        status mapping \
        create-index delete-index reset-index \
        list count get upsert delete-doc \
        search analyze plugins

# ── 帮助 ────────────────────────────────────────────────────

help:
	@echo ""
	@echo "  ES 管理命令 (make -f mk/es.mk <target>)"
	@echo ""
	@echo "  索引管理"
	@echo "    status            查看索引状态（文档数/健康度）"
	@echo "    mapping           查看索引 mapping"
	@echo "    create-index      创建索引（含 dense_vector mapping）"
	@echo "    delete-index      删除索引 ⚠️  不可逆"
	@echo "    reset-index       重建索引（delete → create）"
	@echo ""
	@echo "  文档操作"
	@echo "    list              列出前 10 条文档（仅展示关键字段）"
	@echo "    count             统计文档总数"
	@echo "    get     ID=1      查询指定 ID 的文档"
	@echo "    delete-doc ID=1   删除指定 ID 的文档"
	@echo ""
	@echo "  搜索调试"
	@echo "    search  Q=关键词  BM25 全文搜索（不含向量，用于调试）"
	@echo "    analyze Q=文本    查看分词结果（默认 standard 分词器）"
	@echo "    plugins           查看已安装的 ES 插件"
	@echo ""

# ── 索引管理 ────────────────────────────────────────────────

status:
	@echo "$(STEP) 索引状态:"
	@curl -s $(ES_AUTH) "$(ES_HOST)/_cat/indices/$(ES_INDEX)?v&h=index,health,status,docs.count,store.size"
	@echo ""

mapping:
	@echo "$(STEP) $(ES_INDEX) Mapping:"
	@curl -s $(ES_AUTH) "$(ES_HOST)/$(ES_INDEX)/_mapping"
	@echo ""

# mapping 直接读 $(ES_MAPPING)（与应用 //go:embed 同一份），无内联、无漂移。
# 建物理索引 $(ES_PHYSICAL) 后挂别名 $(ES_INDEX)，与应用 ensureIndex 一致（写/查走别名）。
create-index:
	@echo "$(STEP) 创建物理索引 $(ES_PHYSICAL) + 别名 $(ES_INDEX)..."
	@curl -s $(ES_AUTH) -X PUT "$(ES_HOST)/$(ES_PHYSICAL)" \
	  -H "Content-Type: application/json" \
	  --data-binary @$(ES_MAPPING)
	@echo ""
	@curl -s $(ES_AUTH) -X PUT "$(ES_HOST)/$(ES_PHYSICAL)/_alias/$(ES_INDEX)"
	@echo ""
	@echo "$(INFO) 完成。重新发布文章后会自动写入文档。"

delete-index:
	@echo "$(WARN) 删除物理索引 $(ES_PHYSICAL)（别名 $(ES_INDEX) 随之摘除，数据不可恢复）..."
	@curl -s $(ES_AUTH) -X DELETE "$(ES_HOST)/$(ES_PHYSICAL)"
	@echo ""
	@echo "$(INFO) 索引已删除。"

reset-index:
	@echo "$(STEP) 重建索引..."
	@$(MAKE) -f mk/es.mk delete-index
	@$(MAKE) -f mk/es.mk create-index
	@echo ""
	@echo "$(INFO) 重建完成。请重新发布文章以重建向量索引。"

# ── 文档操作 ────────────────────────────────────────────────

list:
	@echo "$(STEP) 前 10 条文档（id/title/status/author_name）:"
	@curl -s $(ES_AUTH) -X POST "$(ES_HOST)/$(ES_INDEX)/_search" \
	  -H "Content-Type: application/json" \
	  -d '{"query":{"match_all":{}},"size":10,"_source":["id","title","status","author_name","created_at"],"sort":[{"id":"desc"}]}'
	@echo ""

count:
	@echo "$(STEP) 文档总数:"
	@curl -s $(ES_AUTH) "$(ES_HOST)/$(ES_INDEX)/_count"
	@echo ""

get:
	@echo "$(STEP) 查询文档 ID=$(ID):"
	@curl -s $(ES_AUTH) "$(ES_HOST)/$(ES_INDEX)/_doc/$(ID)"
	@echo ""

delete-doc:
	@echo "$(WARN) 删除文档 ID=$(ID)..."
	@curl -s $(ES_AUTH) -X DELETE "$(ES_HOST)/$(ES_INDEX)/_doc/$(ID)"
	@echo ""

# ── 搜索调试 ────────────────────────────────────────────────

# 纯 BM25 文本搜索，不含向量（用于验证索引内容和分词效果）
search:
	@echo "$(STEP) 全文搜索: \"$(Q)\""
	@curl -s $(ES_AUTH) -X POST "$(ES_HOST)/$(ES_INDEX)/_search" \
	  -H "Content-Type: application/json" \
	  -d '{"query":{"bool":{"minimum_should_match":1,"should":[{"match":{"title":"$(Q)"}},{"match":{"abstract":"$(Q)"}}],"filter":{"term":{"status":2}}}},"size":10,"_source":["id","title","abstract","status","author_name"]}'
	@echo ""

# ── 工具 ────────────────────────────────────────────────────

# 查看分词结果: make -f mk/es.mk analyze Q="Go并发编程"
analyze:
	@echo "$(STEP) 分词结果: \"$(Q)\""
	@curl -s $(ES_AUTH) -X POST "$(ES_HOST)/$(ES_INDEX)/_analyze" \
	  -H "Content-Type: application/json" \
	  -d '{"text":"$(Q)"}'
	@echo ""

plugins:
	@echo "$(STEP) ES 已安装插件:"
	@curl -s $(ES_AUTH) "$(ES_HOST)/_cat/plugins?v"
	@echo ""

# make -f mk/es.mk <target>
