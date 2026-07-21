#!/usr/bin/env bash
# 一次性初始化 webook 日志的 ES ILM 策略 + 索引模板（幂等，可重复跑）。
# ES 起来后跑一次（类比 mongo rs.initiate / mk/es.mk create-index）：
#   ES_HOST=http://localhost:9200 ES_PASS=elastic LOG_RETENTION_DAYS=7 ./deploy/elk/es/setup.sh
# 部署机在容器网络内可用 ES_HOST=http://webook-es:9200（或经 socat 桥）。
#
# ⚠ 启动顺序（铁律）：先起栈（COMPOSE_PROFILES=elk ./deploy.sh <env>）→ 再跑本脚本。
#   Kibana 用内置账号 kibana_system 连 ES，而该账号密码只有本脚本 [3/3] 会设；未跑本脚本前
#   Kibana 认证失败会反复重启（restart:always 会在 [3/3] 跑完后自愈，无需手动重启 Kibana）。
set -euo pipefail

ES_HOST="${ES_HOST:-http://localhost:9200}"
ES_USER="${ES_USER:-elastic}"
ES_PASS="${ES_PASS:-elastic}"
RETENTION="${LOG_RETENTION_DAYS:-7}"

curl_es() { curl -sS -u "${ES_USER}:${ES_PASS}" -H 'Content-Type: application/json' "$@"; }

echo "[1/3] PUT ILM policy webook-logs-ilm（${RETENTION}d 后删）"
curl_es -X PUT "${ES_HOST}/_ilm/policy/webook-logs-ilm" -d "{
  \"policy\": {
    \"phases\": {
      \"hot\":    { \"actions\": {} },
      \"delete\": { \"min_age\": \"${RETENTION}d\", \"actions\": { \"delete\": {} } }
    }
  }
}"
echo

echo "[2/3] PUT composable index template webook-logs（webook-logs-* + ECS mapping）"
curl_es -X PUT "${ES_HOST}/_index_template/webook-logs" -d '{
  "index_patterns": ["webook-logs-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0,
      "index.lifecycle.name": "webook-logs-ilm"
    },
    "mappings": {
      "dynamic_templates": [
        { "strings_as_keyword": { "match_mapping_type": "string", "mapping": { "type": "keyword", "ignore_above": 1024 } } }
      ],
      "properties": {
        "@timestamp":                { "type": "date", "format": "epoch_millis||strict_date_optional_time" },
        "message":                   { "type": "text" },
        "log.level":                 { "type": "keyword" },
        "service.name":              { "type": "keyword" },
        "service.environment":       { "type": "keyword" },
        "service.version":           { "type": "keyword" },
        "trace.id":                  { "type": "keyword" },
        "span.id":                   { "type": "keyword" },
        "error.stack_trace":         { "type": "text" },
        "http.response.status_code": { "type": "short" },
        "event.duration":            { "type": "long" },
        "client.ip":                 { "type": "ip" }
      }
    }
  }
}'
echo

echo "[3/3] 设置 kibana_system 密码 = ES_PASS（Kibana 9.x 禁用 elastic 连接，改用内置 kibana_system）"
curl_es -X POST "${ES_HOST}/_security/user/kibana_system/_password" -d "{\"password\":\"${ES_PASS}\"}"
echo
echo "done. Logstash 写 webook-logs-YYYY.MM.dd，模板生效 + ${RETENTION}d 后自动删；Kibana 用 kibana_system 连接。"
