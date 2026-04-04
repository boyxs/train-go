# ============================================================
# ES (Elasticsearch) 管理命令
# 用法: make -f mk/es.mk <target> [ID=xxx] [Q=xxx]
# ============================================================

ES_HOST  := http://localhost:9200
ES_INDEX := article_v1

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
        search search-vec-debug

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
	@echo ""

# ── 索引管理 ────────────────────────────────────────────────

status:
	@echo "$(STEP) 索引状态:"
	@curl -s "$(ES_HOST)/_cat/indices/$(ES_INDEX)?v&h=index,health,status,docs.count,store.size"
	@echo ""

mapping:
	@echo "$(STEP) $(ES_INDEX) Mapping:"
	@curl -s "$(ES_HOST)/$(ES_INDEX)/_mapping"
	@echo ""

create-index:
	@echo "$(STEP) 创建索引 $(ES_INDEX)..."
	@curl -s -X PUT "$(ES_HOST)/$(ES_INDEX)" \
	  -H "Content-Type: application/json" \
	  -d '{"mappings":{"properties":{"id":{"type":"long"},"title":{"type":"text"},"abstract":{"type":"text"},"author_id":{"type":"long"},"author_name":{"type":"keyword"},"status":{"type":"byte"},"created_at":{"type":"date","format":"epoch_millis"},"content_vec":{"type":"dense_vector","dims":1024,"index":true,"similarity":"cosine"}}}}'
	@echo ""
	@echo "$(INFO) 完成。重新发布文章后会自动写入文档。"

delete-index:
	@echo "$(WARN) 删除索引 $(ES_INDEX)（数据不可恢复）..."
	@curl -s -X DELETE "$(ES_HOST)/$(ES_INDEX)"
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
	@curl -s -X POST "$(ES_HOST)/$(ES_INDEX)/_search" \
	  -H "Content-Type: application/json" \
	  -d '{"query":{"match_all":{}},"size":10,"_source":["id","title","status","author_name","created_at"],"sort":[{"id":"desc"}]}'
	@echo ""

count:
	@echo "$(STEP) 文档总数:"
	@curl -s "$(ES_HOST)/$(ES_INDEX)/_count"
	@echo ""

get:
	@echo "$(STEP) 查询文档 ID=$(ID):"
	@curl -s "$(ES_HOST)/$(ES_INDEX)/_doc/$(ID)"
	@echo ""

delete-doc:
	@echo "$(WARN) 删除文档 ID=$(ID)..."
	@curl -s -X DELETE "$(ES_HOST)/$(ES_INDEX)/_doc/$(ID)"
	@echo ""

# ── 搜索调试 ────────────────────────────────────────────────

# 纯 BM25 文本搜索，不含向量（用于验证索引内容和分词效果）
search:
	@echo "$(STEP) 全文搜索: \"$(Q)\""
	@curl -s -X POST "$(ES_HOST)/$(ES_INDEX)/_search" \
	  -H "Content-Type: application/json" \
	  -d '{"query":{"bool":{"minimum_should_match":1,"should":[{"match":{"title":"$(Q)"}},{"match":{"abstract":"$(Q)"}}],"filter":{"term":{"status":2}}}},"size":10,"_source":["id","title","abstract","status","author_name"]}'
	@echo ""

# make -f mk/es.mk <target>
