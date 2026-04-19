# Grafana Provisioning 完整参考

`grafana/` 下所有文件的**字段字典 + 用法说明**。

> - contactpoints.yml（告警通知通道模板）见 [08-alerting-template-reference.md](08-alerting-template-reference.md)
> - **即开即用的完整 example 文件**请直接取 `grafana/examples/`（每种一份 `-example.yml`，已带字段注释，复制 → 去注释 → 改值即可用）
> - 本文定位：概念 / 字段 / 坑 / 工作流的**参考字典**，不是可复制的模板

## 目录

- [一、目录与加载机制](#一目录与加载机制)
- [二、datasources/*.yml（数据源）](#二datasourcesyml数据源)
- [三、dashboards/dashboards.yml（provider 配置）](#三dashboardsdashboardsyml)
- [四、dashboards/*.json（Dashboard Model）](#四dashboardsjsondashboard-model)
- [五、alerting/rules.yml（告警规则）](#五alertingrulesyml告警规则)
- [六、alerting/policies.yml（通知路由）](#六alertingpoliciesyml通知路由)
- [七、mk/grafana.mk（运维 Makefile）](#七mkgrafanamk运维-makefile)
- [八、通用规则](#八通用规则)

---

## 一、目录与加载机制

```
grafana/
├── mk/
│   └── grafana.mk                      # 运维命令（reload / test-email / restart）
├── examples/                           # ⭐ 即开即用的完整注释模板（Grafana 不加载）
│   ├── README.md
│   ├── alerting/{contactpoints,policies,rules}-example.yml
│   ├── dashboards/{dashboards-example.yml,webook-example.json}
│   └── datasources/{prometheus,zipkin,mysql,loki}-example.yml
└── provisioning/                       # 真实配置（Grafana 加载）
    ├── datasources/                    # 数据源（Prometheus / Zipkin / Loki / MySQL...）
    │   ├── prometheus.yml
    │   └── zipkin.yml
    ├── dashboards/                     # Dashboard 面板
    │   ├── dashboards.yml              # provider 配置（扫哪个目录）
    │   ├── webook-overview.json
    │   ├── webook-ops.json
    │   ├── webook-tracing.json
    │   └── linux-host.json
    └── alerting/                       # 告警
        ├── rules.yml                   # 告警规则
        ├── policies.yml                # 通知路由
        └── contactpoints.yml           # 通知通道（见 08）
```

**加载时机对比**：

| 类型 | 启动加载 | `alerting/reload` API | 自动扫描 |
|------|---------|----------------------|---------|
| datasources | ✅ | ❌ | ❌（要 `datasources/reload` API 或 restart） |
| dashboards (JSON) | ✅ | ❌ | ✅（`updateIntervalSeconds: 30` 每 30s 扫） |
| alerting rules | ✅ | ✅ | ❌ |
| alerting policies | ✅ | ✅ | ❌ |
| alerting contactpoints | ✅ | ❌ | ❌ — **必须 restart** |

**结论**：改 contactpoints 或 datasources 都必须 **`docker compose restart grafana`**。其它改动 `make -f mk/grafana.mk reload` 即可。

---

## 二、datasources/*.yml（数据源）

### 2.1 通用结构

```yaml
apiVersion: 1

datasources:
  - name: <UI 显示名>                    # 必填，UI 里看到的 label
    uid: <固定 UID>                      # ⭐ 必填，dashboard/alert 引用用；不填 Grafana 随机生成，跨环境不稳
    type: <prometheus|zipkin|loki|mysql|tempo|elasticsearch|...>
    access: proxy                         # proxy（推荐）/ direct（已 deprecated）
    url: http://<host>:<port>             # 容器网络内用容器名
    isDefault: false                      # Explore 默认选中这个数据源
    editable: false                       # 生产推荐 false，改动走 Git
    orgId: 1                              # 多 org 时才要，默认 1
    version: 1                            # provisioning 版本号（改了递增）
    basicAuth: false
    basicAuthUser: ""
    withCredentials: false
    jsonData: {}                          # 类型特定配置（见下）
    secureJsonData: {}                    # 凭证类（加密存 grafana.db，UI 不回显）
    deleteDatasources: []                 # 要删除的数据源列表
```

### 2.2 Prometheus 完整 example

```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    uid: prometheus                       # ⭐ dashboard JSON 里 datasource.uid=prometheus 引用
    type: prometheus
    access: proxy
    url: http://webook-prometheus:9090    # docker compose 服务名
    isDefault: true
    editable: false
    jsonData:
      httpMethod: POST                    # 长查询用 POST 避免 URL 过长
      timeInterval: "15s"                 # 默认采集间隔（与 prometheus.yml 的 scrape_interval 对齐）
      queryTimeout: "60s"
      manageAlerts: false                 # 是否让 Grafana 管 Prometheus 原生告警规则
      prometheusType: Prometheus          # Prometheus / Mimir / Cortex / Thanos
      prometheusVersion: 3.4.1
      cacheLevel: Medium                  # None / Low / Medium / High
      # 与 Tempo/Zipkin 联动：Histogram exemplar 点击跳 trace
      exemplarTraceIdDestinations:
        - name: trace_id
          datasourceUid: zipkin           # 本项目是 Zipkin
          # urlDisplayLabel: "Trace"      # 自定义显示文本
```

### 2.3 Zipkin 完整 example

```yaml
apiVersion: 1
datasources:
  - name: Zipkin
    uid: zipkin
    type: zipkin
    access: proxy
    url: http://webook-zipkin:9411
    isDefault: false
    editable: false
    jsonData:
      # trace → metrics 联动（在 Zipkin trace 详情点"View metrics"）
      tracesToMetrics:
        datasourceUid: prometheus
        # tags 透传：trace span 上的哪些标签作为 Prometheus 查询的标签筛选
        tags:
          - key: service.name
            value: service
        # 预置查询：点击直接跳
        queries:
          - name: "请求总量"
            query: 'sum(rate(webook_http_requests_total{$__tags}[5m]))'
      # trace → logs 联动（接了 Loki 才用）
      # tracesToLogsV2:
      #   datasourceUid: loki
      #   tags: ["service.name", "trace_id"]
      #   filterByTraceID: true
      #   filterBySpanID: false
      # 服务依赖图（需要 Tempo 的 metrics_generator，Zipkin 不支持）
      # serviceMap:
      #   datasourceUid: prometheus
```

### 2.4 MySQL 完整 example（业务库查询）

```yaml
apiVersion: 1
datasources:
  - name: WebookDB
    uid: webook-mysql
    type: mysql
    url: webook-mysql:3306                # 注意 mysql 不用 http://
    user: grafana_ro                      # 必须只读账号
    jsonData:
      database: webook
      maxOpenConns: 10
      maxIdleConns: 2
      connMaxLifetime: 14400              # 秒
      tlsAuth: false
    secureJsonData:
      password: "${GRAFANA_MYSQL_PASSWORD}"  # 从 docker env 注入
```

### 2.5 Loki 完整 example（日志，未来接入）

```yaml
apiVersion: 1
datasources:
  - name: Loki
    uid: loki
    type: loki
    access: proxy
    url: http://loki:3100
    jsonData:
      maxLines: 1000
      # 日志里提取 trace_id 跳 Zipkin
      derivedFields:
        - name: TraceID
          matcherRegex: "trace_id=(\\w+)"
          url: "$${__value.raw}"
          datasourceUid: zipkin
          urlDisplayLabel: "View trace"
```

### 2.6 常见坑

| 坑 | 修 |
|----|---|
| dashboard 报数据源 "not found" | `uid` 不匹配；固定 uid 在所有环境保持一致 |
| 容器间连不上 | url 用**容器名** 不用 `localhost` |
| 换后端要改一堆 dashboard | 固定 uid（`prometheus` / `zipkin`），换 datasource 实例时保持 uid 不变 |
| 密码明文泄露 | 放 `secureJsonData` + 从 env var 注入，不在 yaml 里写死 |

---

## 三、dashboards/dashboards.yml

这是**provider 配置**（告诉 Grafana 去哪里扫 JSON dashboard），不是 dashboard 本身。

```yaml
apiVersion: 1

providers:
  - name: 'webook'                        # provider 显示名
    orgId: 1
    folder: 'Webook'                      # Grafana UI 里的 folder 名
    folderUid: webook                     # ⭐ 固定 folder uid，便于权限引用
    type: file                            # 从文件加载
    disableDeletion: false                # 生产推荐 true（UI 删了重启会回来）
    editable: true                        # 生产推荐 false（改动走 Git）
    allowUiUpdates: true                  # 生产推荐 false（UI 改不写回文件，重启丢失）
    updateIntervalSeconds: 30             # 多久扫一次目录找新/改文件
    options:
      path: /etc/grafana/provisioning/dashboards    # 容器内目录
      foldersFromFilesStructure: true     # 子目录自动变成 Grafana folder
```

**生产三件套（防误改）**：
```yaml
disableDeletion: true
editable: false
allowUiUpdates: false
```

三件套全关后，UI 上 dashboard 是**完全只读**，修改必须通过文件。详见 `docs/grafana/06-production-workflow.md`。

---

## 四、dashboards/*.json（Dashboard Model）

Dashboard 的 JSON schema 很大（100+ 字段）。下面是**能跑起来的最小完整骨架 + 最常用字段**。

### 4.1 骨架

```json
{
  "title": "Webook / Tracing",            // 必填
  "uid": "webook-tracing",                // ⭐ 必填，固定 + 手动命名（不要让 Grafana 自动生成）
  "tags": ["webook", "tracing"],          // 分类 tag，便于搜索
  "timezone": "browser",                  // browser / utc
  "schemaVersion": 39,                    // Grafana 导出时填，手写可省（Grafana 会补）
  "version": 1,                           // dashboard 版本号
  "editable": false,                      // 生产 false
  "refresh": "30s",                       // 默认自动刷新间隔，建议 ≥ 30s
  "time": {"from": "now-1h", "to": "now"},
  "timepicker": {
    "refresh_intervals": ["5s","10s","30s","1m","5m"]
  },
  "templating": {"list": []},             // 变量（见下）
  "annotations": {"list": []},            // 事件标注
  "panels": [],                           // 面板数组（见下）
  "links": []                             // 顶部跳转链接
}
```

### 4.2 变量（Templating）

```json
"templating": {
  "list": [
    {
      "name": "instance",                 // ⭐ 用 $instance 引用
      "label": "实例",
      "type": "query",                    // query / custom / datasource / interval / textbox / adhoc / constant
      "datasource": {"type": "prometheus", "uid": "prometheus"},
      "query": "label_values(up, instance)",
      "refresh": 2,                       // 0=从不 / 1=dashboard 加载 / 2=时间范围变更
      "includeAll": true,
      "multi": true,
      "allValue": ".*",                   // All 选项对应的值（正则）
      "sort": 1                           // 0=无序 / 1=字母升 / 2=字母降 / 3=数字升 / ...
    }
  ]
}
```

### 4.3 Panel 常见类型（按使用频率）

#### Time series（时间序列图）

```json
{
  "type": "timeseries",
  "title": "HTTP QPS（按路径）",
  "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8},   // 24 列 grid，w 宽度，h 高度
  "datasource": {"type": "prometheus", "uid": "prometheus"},
  "targets": [
    {
      "expr": "sum by (pattern) (rate(webook_http_requests_total[$__rate_interval]))",
      "refId": "A",
      "legendFormat": "{{pattern}}"        // {{label}} 作为图例
    }
  ],
  "fieldConfig": {
    "defaults": {
      "unit": "reqps",                    // 单位，参见下方表格
      "decimals": 2,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          {"color": "green", "value": null},
          {"color": "yellow", "value": 100},
          {"color": "red", "value": 500}
        ]
      }
    }
  },
  "options": {
    "legend": {
      "displayMode": "table",             // list / table / hidden
      "placement": "bottom",              // bottom / right
      "calcs": ["last", "max", "mean"]    // 图例表格的统计项
    },
    "tooltip": {"mode": "multi"}          // single / multi / none
  }
}
```

#### Stat（单一数字）

```json
{
  "type": "stat",
  "title": "HTTP 5xx 错误率",
  "gridPos": {"x": 0, "y": 0, "w": 6, "h": 6},
  "datasource": {"type": "prometheus", "uid": "prometheus"},
  "targets": [
    {
      "expr": "sum(rate(webook_http_requests_total{status=~\"5..\"}[5m])) / sum(rate(webook_http_requests_total[5m]))",
      "refId": "A",
      "instant": true                     // Stat 用 instant 查询，只取一个点
    }
  ],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",              // 0.01 显示为 1%（注意：不是 percent）
      "decimals": 3,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          {"color": "green", "value": null},
          {"color": "red", "value": 0.01}
        ]
      }
    }
  },
  "options": {
    "colorMode": "value",                 // value / background / none
    "graphMode": "area",                  // none / area 迷你图
    "textMode": "value_and_name",         // value / name / value_and_name / none
    "justifyMode": "auto"
  }
}
```

#### Gauge（仪表盘）

```json
{
  "type": "gauge",
  "title": "CPU 使用率",
  "datasource": {"type": "prometheus", "uid": "prometheus"},
  "targets": [{"expr": "rate(process_cpu_seconds_total[5m])", "refId": "A"}],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "min": 0,
      "max": 1,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          {"color": "green", "value": null},
          {"color": "yellow", "value": 0.6},
          {"color": "red", "value": 0.85}
        ]
      }
    }
  }
}
```

#### Table（表格）

```json
{
  "type": "table",
  "title": "实例列表",
  "datasource": {"type": "prometheus", "uid": "prometheus"},
  "targets": [
    {"expr": "up", "refId": "A", "instant": true, "format": "table"}
  ],
  "fieldConfig": {
    "defaults": {"align": "auto"}
  }
}
```

#### Text（Markdown 说明）

```json
{
  "type": "text",
  "title": "使用说明",
  "gridPos": {"x": 0, "y": 0, "w": 24, "h": 4},
  "options": {
    "mode": "markdown",                   // markdown / html / code
    "content": "## 标题\n- 第一条\n- 第二条"
  }
}
```

#### Traces（trace 列表）

```json
{
  "type": "traces",
  "title": "最近的 traces",
  "datasource": {"type": "zipkin", "uid": "zipkin"},
  "targets": [
    {
      "refId": "A",
      "queryType": "traceqlSearch",
      "serviceName": "webook"
    }
  ]
}
```

### 4.4 常用单位速查

| unit | 显示效果 | 用途 |
|------|---------|------|
| `none` | 原值 | 计数 |
| `short` | `1.2k` / `3.4M` | 大数字 |
| `percent` | `95` → `95%` | 原值已是百分比 |
| `percentunit` | `0.95` → `95%` | 原值是 0-1 的比率 |
| `s` | `1.5` → `1.5s` | 秒（自动转 ms/ns/min/h）|
| `ms` | `500` → `500ms` | 毫秒 |
| `bytes` | `1024` → `1.00 KiB` | 二进制字节（IEC）|
| `decbytes` | `1000` → `1.00 kB` | 十进制字节（SI）|
| `reqps` | `1.23` → `1.23 req/s` | 请求率 |
| `dateTimeAsIso` | `2026-04-19T10:00:00` | ISO 8601 |

### 4.5 `gridPos` 布局系统

- 宽 24 列，高无限
- `{"x": 0, "y": 0, "w": 24, "h": 8}` = 左上角满宽 8 高
- 三栏：`w: 8` 三个；两栏：`w: 12` 两个；四栏：`w: 6` 四个

### 4.6 完整最小 example（可直接丢进 dashboards/ 运行）

```json
{
  "title": "Webook / 示例",
  "uid": "webook-example",
  "tags": ["webook", "example"],
  "timezone": "browser",
  "refresh": "30s",
  "time": {"from": "now-1h", "to": "now"},
  "templating": {"list": []},
  "annotations": {"list": []},
  "editable": false,
  "panels": [
    {
      "type": "text",
      "title": "说明",
      "gridPos": {"x": 0, "y": 0, "w": 24, "h": 3},
      "options": {"mode": "markdown", "content": "这是一个参考 dashboard"}
    },
    {
      "type": "stat",
      "title": "QPS",
      "gridPos": {"x": 0, "y": 3, "w": 12, "h": 6},
      "datasource": {"type": "prometheus", "uid": "prometheus"},
      "targets": [
        {"expr": "sum(rate(webook_http_requests_total[5m]))", "refId": "A"}
      ],
      "fieldConfig": {"defaults": {"unit": "reqps"}}
    },
    {
      "type": "timeseries",
      "title": "QPS 趋势",
      "gridPos": {"x": 12, "y": 3, "w": 12, "h": 6},
      "datasource": {"type": "prometheus", "uid": "prometheus"},
      "targets": [
        {
          "expr": "sum(rate(webook_http_requests_total[$__rate_interval]))",
          "refId": "A",
          "legendFormat": "total"
        }
      ],
      "fieldConfig": {"defaults": {"unit": "reqps"}}
    }
  ],
  "links": [
    {
      "title": "Webook Overview",
      "url": "/d/webook-overview",
      "type": "link",
      "icon": "dashboard"
    }
  ]
}
```

### 4.7 入仓前清理脚本

UI 导出的 JSON 含 `id` / `version` 会有冲突，入仓前清洗：

```bash
jq 'del(.id, .version) | .uid = "webook-xxx"' raw.json > grafana/provisioning/dashboards/xxx.json
```

---

## 五、alerting/rules.yml（告警规则）

### 5.1 通用结构

```yaml
apiVersion: 1

groups:
  - orgId: 1
    name: <组名>                          # 同组告警一起评估
    folder: <Grafana folder 名>           # 必须是存在的 folder（不存在会报错）
    interval: 1m                          # 评估周期，生产 1m，业务告警可 5m
    rules:
      - uid: <固定 uid>                   # ⭐ 必填，便于跨环境同步
        title: <告警名>                   # UI / 邮件里看到的名字
        condition: <refId>                # 指定哪个 refId 是触发条件（Threshold 节点）
        for: 5m                           # 持续多久才 firing（避免抖动）
        noDataState: <NoData|OK|Alerting> # 查不到数据时的状态
        execErrState: <Error|OK|Alerting> # 评估出错时的状态
        labels:                           # 供 policies.yml 路由匹配
          severity: critical              # critical / high / medium / low
          team: backend
        annotations:                      # 告警详情（邮件正文用）
          summary: "xxx 出问题了"
          description: "详细描述可多行"
          runbook_url: "https://wiki/runbook/xxx"
        data:                             # 评估链路（A → B → C → ...）
          - refId: A                      # 每步一个 refId
            relativeTimeRange: {from: 600, to: 0}  # 查过去 600 秒
            datasourceUid: <ds-uid>
            model:                        # 查询或表达式
              ...
```

### 5.2 评估链路 data[] 详解

**核心规则**：链路必须终止在一个 **Threshold 节点**，且该节点输入必须是**单值**（reduce 后）。

典型链路模式：

```
Query (A)    → Reduce (B) → Threshold (C)             # 最简单
Query (A,B)  → Reduce (C,D) → Math (E) → Threshold (F)  # 算比例
Query (A)    → Reduce (B) → Classic Condition         # 旧版 Grafana 风格
```

#### Query 节点（从数据源查）

```yaml
- refId: A
  queryType: ""                           # 留空即可
  relativeTimeRange:
    from: 600                             # 秒，相对于 now
    to: 0
  datasourceUid: prometheus               # ⭐ 必须匹配已注册的数据源 uid
  model:
    expr: 'sum(rate(webook_http_requests_total[5m]))'   # PromQL / LogQL / SQL
    refId: A                              # 与外层一致
    instant: false                        # true=查单点；false=查时间范围
    intervalMs: 1000
    maxDataPoints: 43200
```

#### Reduce 节点（压成单值）

```yaml
- refId: B
  datasourceUid: __expr__                 # 固定值：内置表达式引擎
  model:
    type: reduce                          # 节点类型
    expression: A                         # 引用哪个 refId 的结果
    reducer: last                         # last / mean / min / max / sum / count / median
    refId: B
    # 可选：设置 reduce 前对源做预处理
    settings:
      mode: dropNN                        # 丢弃 NaN/Inf 点；replaceNN=替换为下面的值
      # replaceWithValue: 0
```

#### Math 节点（算术）

```yaml
- refId: E
  datasourceUid: __expr__
  model:
    type: math
    expression: '$C / $D * 100'           # $<refId> 引用（必须是 reduce 后的单值）
    refId: E
```

#### Threshold 节点（判定触发）

```yaml
- refId: F
  datasourceUid: __expr__
  model:
    type: threshold
    expression: E                         # 引用上一个节点（必须单值）
    refId: F
    conditions:
      - evaluator:
          type: gt                        # gt / lt / within_range / outside_range
          params: [1]                     # gt/lt 一个参数；within/outside 两个
        # Grafana 9+ 这里还可以设 unloadEvaluator（告警恢复阈值，防抖）
        unloadEvaluator:
          type: lt
          params: [0.95]
```

#### Classic Condition 节点（旧风格，一个节点搞定 reduce + threshold）

```yaml
- refId: B
  datasourceUid: __expr__
  model:
    type: classic_conditions
    refId: B
    conditions:
      - type: query
        query: {params: [A, 5m, now]}
        reducer: {type: last, params: []}
        evaluator: {type: gt, params: [0.5]}
        operator: {type: and}             # 多条件时用
```

### 5.3 annotations 里的模板变量

告警 summary / description 可以用 Go template 访问查询结果：

```yaml
annotations:
  summary: "{{ $labels.instance }} CPU {{ $values.A.Value | humanizePercentage }}"
  description: |
    规则：{{ $labels.alertname }}
    严重：{{ $labels.severity }}
    实例：{{ $labels.instance }}
    当前值：{{ $values.B.Value }}
    阈值：1.0
```

**可用变量**：
- `$labels` — 当前告警实例的 labels
- `$values` — map[refId]Value，每个 Value 有 `.Value`（float） `.Labels`
- `$value` — 废弃，不要用
- 函数：`humanize` / `humanizePercentage` / `humanizeDuration` / `printf`

### 5.4 完整 example

```yaml
apiVersion: 1

groups:
  - orgId: 1
    name: webook-core
    folder: Webook
    interval: 1m
    rules:

      # 规则 1：简单阈值（单查询 → reduce → threshold）
      - uid: webook-up
        title: Webook 服务不可达
        condition: C                      # threshold 是 refId=C 的节点
        for: 1m
        noDataState: NoData
        execErrState: Error
        labels:
          severity: critical
          team: backend
        annotations:
          summary: "Webook 实例 {{ $labels.instance }} 不可达"
          runbook_url: "https://wiki/runbook/webook-down"
        data:
          - refId: A
            relativeTimeRange: {from: 60, to: 0}
            datasourceUid: prometheus
            model:
              expr: 'up{job="webook-app"}'
              refId: A
              instant: true
          - refId: B
            datasourceUid: __expr__
            model:
              type: reduce
              expression: A
              reducer: last
              refId: B
          - refId: C
            datasourceUid: __expr__
            model:
              type: threshold
              expression: B
              conditions:
                - evaluator: {type: lt, params: [1]}
              refId: C

      # 规则 2：比率型告警（两查询 → 两 reduce → math → threshold）
      - uid: webook-5xx-rate
        title: HTTP 5xx 错误率 > 1%
        condition: F
        for: 5m
        noDataState: OK
        execErrState: Error
        labels:
          severity: high
          team: backend
        annotations:
          summary: "5xx 错误率 {{ $values.E.Value | humanizePercentage }}"
          runbook_url: "https://wiki/runbook/webook-5xx"
        data:
          - refId: A                      # 分子
            relativeTimeRange: {from: 600, to: 0}
            datasourceUid: prometheus
            model:
              expr: 'sum(rate(webook_http_requests_total{status=~"5.."}[5m]))'
              refId: A
          - refId: B                      # 分母
            relativeTimeRange: {from: 600, to: 0}
            datasourceUid: prometheus
            model:
              expr: 'sum(rate(webook_http_requests_total[5m]))'
              refId: B
          - refId: C                      # 分子 reduce
            datasourceUid: __expr__
            model: {type: reduce, expression: A, reducer: last, refId: C}
          - refId: D                      # 分母 reduce
            datasourceUid: __expr__
            model: {type: reduce, expression: B, reducer: last, refId: D}
          - refId: E                      # 比例
            datasourceUid: __expr__
            model: {type: math, expression: '$C / $D', refId: E}
          - refId: F                      # 阈值
            datasourceUid: __expr__
            model:
              type: threshold
              expression: E
              conditions:
                - evaluator: {type: gt, params: [0.01]}
              refId: F
```

### 5.5 常见错误

| 错误 | 原因 | 修 |
|------|------|---|
| `invalid format of evaluation results for the alert definition X: looks like time series data, only reduced data can be alerted on` | Threshold 直接接 Query（时间序列） | 中间加 Reduce 节点 |
| `failed to expand template ... can't convert <nil> to float: humanizePercentage` | `$values.X.Value` 在 NoData 时是 nil | annotation 包 `{{ if $values.X }}...{{ end }}` |
| folder not found | folder 名拼错或不存在 | 改 folder 字段 或先手动在 UI 建 |
| refId 冲突 | 同一 rule 的 data[] 里 refId 重复 | 每个节点 refId 唯一 |

---

## 六、alerting/policies.yml（通知路由）

**作用**：告警触发后走哪个 contact point，怎么分组 / 去重 / 限流。

### 6.1 完整结构

```yaml
apiVersion: 1

policies:
  - orgId: 1
    receiver: <默认 contact point>        # 根 policy 必填，没匹配到子路由就用这个
    group_by: ['alertname', 'severity']   # 按哪些 label 分组（同组合并发一封）
    group_wait: 30s                       # 新组首次告警等多久（攒批）
    group_interval: 5m                    # 同组后续告警间隔
    repeat_interval: 4h                   # 持续 firing 的重复提醒间隔
    mute_time_intervals: []               # 引用 muteTimes 名称，该时段不发
    # 子路由（按 label 匹配路由到不同 receiver）
    routes:
      - receiver: <contact point 名>
        matchers:                         # 匹配条件（AND）
          - severity = critical
        group_wait: 0s                    # critical 立即发
        group_interval: 1m
        repeat_interval: 1h
        continue: false                   # false=匹配到就停；true=继续匹配后面的 route
        object_matchers:                  # 等价 matchers，JSON 风格
          - [team, =, data]

# 静默时间段（可选）
muteTimes:
  - name: weekend
    time_intervals:
      - weekdays: ["saturday", "sunday"]
      - times:
          - start_time: "00:00"
            end_time: "23:59"
        location: "Asia/Shanghai"
```

### 6.2 matcher 语法

- `label = value` — 等于
- `label != value` — 不等
- `label =~ regex` — 正则匹配
- `label !~ regex` — 正则不匹配

多条件在同一 `matchers[]` 里是 **AND**。用 OR 需要拆两个 route。

### 6.3 group_by 深入

假设 group_by: `["alertname", "severity"]`：
- alert1: `{alertname: UP, severity: critical, instance: A}`
- alert2: `{alertname: UP, severity: critical, instance: B}`
- alert3: `{alertname: UP, severity: high, instance: A}`

→ 分成 2 组：`{UP, critical}`（含 alert1+alert2，一封邮件）和 `{UP, high}`（含 alert3）。

特殊值：
- `group_by: [...]` 为空数组：每个告警单独一组（最吵）
- `group_by: ['...']` 含 `'...'` 字符串：按所有 label 分组
- `group_by: ['alertname']` 推荐起点

### 6.4 生产推荐 timing

| 级别 | group_wait | group_interval | repeat_interval |
|------|-----------|----------------|-----------------|
| critical（服务宕） | 0s | 1m | 1h |
| high（业务受影响） | 30s | 5m | 4h |
| medium（性能下降） | 1m | 10m | 12h |
| low（趋势警告） | 5m | 30m | 24h |

### 6.5 完整 example

```yaml
apiVersion: 1

policies:
  - orgId: 1
    receiver: webook-email                # 默认通道
    group_by: ['alertname', 'severity']
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 4h
    routes:
      # critical 立即发
      - receiver: webook-email
        matchers:
          - severity = critical
        group_wait: 0s
        group_interval: 1m
        repeat_interval: 1h
        continue: false

      # 数据团队的告警走别的通道（示例）
      # - receiver: data-team-dingtalk
      #   matchers:
      #     - team = data
      #   continue: false

      # 白天不发 low 告警（示例）
      # - receiver: webook-email
      #   matchers:
      #     - severity = low
      #   mute_time_intervals: ['work-hours']

# muteTimes:
#   - name: work-hours
#     time_intervals:
#       - weekdays: ["monday:friday"]
#         times:
#           - start_time: "09:00"
#             end_time: "18:00"
#         location: "Asia/Shanghai"
```

---

## 七、mk/grafana.mk（运维 Makefile）

### 7.1 命令速查

```bash
cd grafana

make -f mk/grafana.mk help              # 列出所有命令
make -f mk/grafana.mk reload            # reload alerting（改规则/路由时用）
make -f mk/grafana.mk reload-datasources # reload 数据源
make -f mk/grafana.mk reload-dashboards  # reload dashboards（通常不用）
make -f mk/grafana.mk reload-all        # 全 reload
make -f mk/grafana.mk test-email        # 发测试告警邮件
make -f mk/grafana.mk restart           # 重启容器（改 contactpoints 必用）
```

### 7.2 环境变量覆盖

```bash
# 换 host
GRAFANA_HOST=grafana.prod.com:3000 make -f mk/grafana.mk reload

# 换凭证
GRAFANA_USER=svc-bot GRAFANA_PASS=xxx make -f mk/grafana.mk reload
```

### 7.3 添加新命令模板

```makefile
.PHONY: my-new-cmd
my-new-cmd:
	@echo "doing xxx..."
	@curl -sS -X POST $(AUTH) $(GRAFANA_URL)/api/xxx
	@echo "✅ done"
```

---

## 八、通用规则

### 8.1 固定 uid（所有 provisioning 资源）

- **必须手动指定** `uid`，不让 Grafana 随机生成
- dashboard uid 形如 `webook-<scope>`（`webook-overview` / `webook-tracing`）
- datasource uid 用类型名或简称（`prometheus` / `zipkin`）
- alert rule uid 形如 `<project>-<metric>-<threshold>`（`webook-5xx-rate`）

### 8.2 改动流程（生产）

```
编辑本地 yml/json
   ↓
git commit + PR
   ↓
merge master
   ↓
scp / rsync 到服务器
   ↓
视文件类型选：
   - alerting rules/policies → make reload
   - contactpoints / datasources → make restart
   - dashboards → 自动扫（30s 内），或 make reload-dashboards
   ↓
验证：
   - make test-email（告警通道）
   - curl API 查状态
   - Grafana UI 人眼确认
```

### 8.3 生产加固 checklist

- [ ] 所有 uid 手动固定，不随机
- [ ] dashboards.yml 三件套：`disableDeletion=true` `editable=false` `allowUiUpdates=false`
- [ ] datasources `editable: false`
- [ ] 密码 / token 走 `secureJsonData` + 环境变量，不写死 yaml
- [ ] admin 密码换强密码；关匿名注册；考虑 SSO
- [ ] 所有告警 rule 带 `runbook_url` annotation
- [ ] 告警带 `team` label 便于路由到值班
- [ ] 关键告警带 `unloadEvaluator`（恢复阈值防抖）
- [ ] SMTP 从环境变量注入，不在 compose 里写死
- [ ] `.env` 入 `.gitignore`，不要入库

### 8.4 相关文档索引

| 文档 | 聚焦点 |
|------|--------|
| [01-concepts.md](01-concepts.md) | Grafana 是什么、架构 |
| [02-deployment.md](02-deployment.md) | 部署、SMTP 环境变量 |
| [03-datasources.md](03-datasources.md) | 数据源总览 |
| [04-dashboards.md](04-dashboards.md) | Dashboard 设计理念 |
| [05-alerting.md](05-alerting.md) | 告警整体架构 |
| [06-production-workflow.md](06-production-workflow.md) | Dashboard-as-Code 生产流程 |
| [07-best-practices.md](07-best-practices.md) | 最佳实践 |
| [08-alerting-template-reference.md](08-alerting-template-reference.md) | **Contact point 邮件模板完整参考** |
| **09-provisioning-reference.md** | **本文：datasources / dashboards / rules / policies / mk 完整模板参考** |
