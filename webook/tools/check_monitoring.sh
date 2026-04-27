#!/bin/bash
# 监控配置校验：YAML/JSON 语法 + 元素数量。Prom/Grafana 启动前的"单元测试"。
# 用法：bash tools/check_monitoring.sh
set -e

# 转 Windows 原生路径（python.exe 不识别 mingw 的 /c/...）
ROOT="$(cygpath -w "$(cd "$(dirname "$0")/../.." && pwd)" 2>/dev/null || echo "C:/Go/work")"
PROM_RULES="$ROOT/deploy/prometheus/rules/webook-jobs.rules.yml"
PROM_RULES_EX="$ROOT/deploy/prometheus/examples/recording-rules-example.yml"
GRAFANA_ALERTS="$ROOT/deploy/grafana/provisioning/alerting/webook-jobs.yml"
GRAFANA_ALERTS_EX="$ROOT/deploy/grafana/examples/alerting/rules-recording-example.yml"
GRAFANA_DASH="$ROOT/deploy/grafana/provisioning/dashboards/webook-jobs.json"
GRAFANA_DASH_EX="$ROOT/deploy/grafana/examples/dashboards/webook-jobs-example.json"

python -c "
import yaml, io
with io.open(r'$PROM_RULES', encoding='utf-8') as f:
    data = yaml.safe_load(f)
records = sum(len([r for r in g['rules'] if 'record' in r]) for g in data['groups'])
assert records == 8, f'expected 8 records, got {records}'
print('OK prom rules: 8 recording rules')
"

python -c "
import yaml, io
with io.open(r'$PROM_RULES_EX', encoding='utf-8') as f:
    content = f.read()
yaml.safe_load(content)
comments = sum(1 for line in content.splitlines() if line.strip().startswith('#'))
assert comments >= 20, f'expected >= 20 comments, got {comments}'
print(f'OK prom rules-example: {comments} comment lines')
"

python -c "
import yaml, io
with io.open(r'$GRAFANA_ALERTS', encoding='utf-8') as f:
    data = yaml.safe_load(f)
rules = []
for g in data['groups']:
    rules.extend(g['rules'])
assert len(rules) == 6, f'expected 6 alerts, got {len(rules)}'
for rule in rules:
    refids = {d['refId'] for d in rule['data']}
    assert len(refids) >= 3, f\"alert {rule['title']} expected >= 3 refIds, got {refids}\"
print('OK grafana alerts: 6 alerts with Q/Reduce/Threshold chains')
"

python -c "
import yaml, io
with io.open(r'$GRAFANA_ALERTS_EX', encoding='utf-8') as f:
    content = f.read()
yaml.safe_load(content)
comments = sum(1 for line in content.splitlines() if line.strip().startswith('#'))
assert comments >= 20, f'expected >= 20 comments, got {comments}'
print(f'OK grafana alerts-example: {comments} comment lines')
"

python -c "
import json, io
with io.open(r'$GRAFANA_DASH', encoding='utf-8') as f:
    data = json.load(f)
panels = data.get('panels', [])
assert len(panels) == 14, f'expected 14 panels, got {len(panels)}'
for p in panels:
    assert 'title' in p, f'panel missing title: {p}'
    assert 'gridPos' in p, f'panel {p[\"title\"]} missing gridPos'
print('OK grafana dashboard: 14 panels')
"

python -c "
import json, io
with io.open(r'$GRAFANA_DASH_EX', encoding='utf-8') as f:
    data = json.load(f)
assert len(data.get('panels', [])) == 14, 'expected 14 panels'
print('OK grafana dashboard-example: 14 panels')
"

echo ''
echo '==== ALL MONITORING CONFIG CHECKS PASSED ===='
