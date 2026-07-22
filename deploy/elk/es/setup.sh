#!/usr/bin/env bash
# 初始化 webook 日志的 ES ILM 策略 + 索引模板 + kibana_system 密码（幂等，可重复跑）。
# 起栈时由 webook-es-setup 容器自动执行；以下是手动重跑（改保留期等）的方式。
#
# ⚠ webook-es 的 9200 只在容器网络内（base compose 不发布宿主端口），宿主机直连 localhost:9200 会 refused。
#   服务器手动跑：喂进 ES 容器（容器内 localhost:9200 即 ES，密码取自带 ELASTIC_PASSWORD）
#     docker cp ./setup.sh webook-es:/tmp/setup.sh
#     docker exec -e LOG_RETENTION_DAYS=30 webook-es \
#       bash -c 'ES_HOST=http://localhost:9200 ES_PASS="$ELASTIC_PASSWORD" bash /tmp/setup.sh'
#   LOG_RETENTION_DAYS 按 env：prod 30 / staging 14 / dev 7（默认 7）。
set -euo pipefail

ES_HOST="${ES_HOST:-http://localhost:9200}"
ES_USER="${ES_USER:-elastic}"
ES_PASS="${ES_PASS:-elastic}"
RETENTION="${LOG_RETENTION_DAYS:-7}"

curl_es() { curl -sS -u "${ES_USER}:${ES_PASS}" -H 'Content-Type: application/json' "$@"; }

echo "[1/3] PUT ILM policy logs-webook-ilm（${RETENTION}d 后删）"
curl_es -X PUT "${ES_HOST}/_ilm/policy/logs-webook-ilm" -d "{
  \"policy\": {
    \"phases\": {
      \"hot\":    { \"actions\": {} },
      \"delete\": { \"min_age\": \"${RETENTION}d\", \"actions\": { \"delete\": {} } }
    }
  }
}"
echo

echo "[2/3] PUT composable index template logs-webook（logs-webook-* + ECS mapping）"
# priority 200：盖过 ES 内置 logs 模板（logs-*-*，priority 100，只建 data stream），
# 让 logs-webook-* 落成普通按天索引而非 data stream（Logstash 直写普通索引，非 data stream）。
curl_es -X PUT "${ES_HOST}/_index_template/logs-webook" -d '{
  "index_patterns": ["logs-webook-*"],
  "priority": 200,
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0,
      "index.lifecycle.name": "logs-webook-ilm"
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
        "error":                     { "type": "text" },
        "stack_trace":               { "type": "text" },
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
echo "done. Logstash 写 logs-webook-YYYY.MM.dd，模板生效 + ${RETENTION}d 后自动删；Kibana 用 kibana_system 连接。"
