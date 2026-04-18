# Dashboard 与 Panel 设计

## 一、Dashboard 模型

一个 Dashboard 在 Grafana 内部是一份 JSON，结构：

```
Dashboard
├── meta（标题、tags、timezone）
├── time（默认时间范围）
├── refresh（自动刷新间隔）
├── templating.list（变量）
├── annotations.list（事件标注）
├── panels[]
│   └── Panel
│       ├── type（timeseries / stat / gauge / table / ...）
│       ├── datasource
│       ├── targets[]（查询，每条产出一组数据）
│       ├── fieldConfig（颜色 / 单位 / 阈值）
│       └── options（图表特有选项）
└── links（外部链接）
```

**核心 mental model**：Panel = 1 个或 N 个查询 → 1 张图。

## 二、常用 Panel 类型

| 类型 | 用途 | 适合的查询 |
|------|------|-----------|
| **Time series** | 时间趋势曲线 | `rate(...)`、`gauge` 类指标 |
| **Stat** | 单一大数字 + 迷你 sparkline | `sum(...)`、当前 QPS、错误率 |
| **Gauge** | 仪表盘（带阈值色块） | 利用率类（CPU% / 内存%） |
| **Bar gauge** | 横/竖条形图 | TopN（top 5 接口、top 10 IP） |
| **Table** | 表格 | 实例列表 + 多指标 |
| **Pie chart** | 饼图 | 占比类（按状态码分布） |
| **Heatmap** | 热力图 | Histogram 分布随时间变化 |
| **Histogram** | 频率分布 | 单时间点的分布 |
| **State timeline** | 状态条带 | up/down 状态、告警状态 |
| **Logs** | 日志展示 | Loki 查询结果 |
| **Traces** | Trace 火焰图 | Tempo / Jaeger / Zipkin |
| **Bar chart** | 柱状图 | 离散维度对比 |
| **Geomap** | 地图 | 带地理坐标的指标 |
| **Text** | 文本 / Markdown | 说明、链接、值班信息 |

**新手最常用三件套**：Time series（趋势）+ Stat（关键数字）+ Table（明细）。

## 三、查询编辑器关键字段

每个 Panel 的 `targets` 数组里每条 query：

| 字段 | 含义 |
|------|------|
| **expr / query** | 查询语句（PromQL / LogQL / SQL） |
| **legend** | 图例显示模板，如 `{{instance}}`、`{{method}} {{pattern}}` |
| **interval** | 步长（用 Grafana 的 `$__interval` 自适应最常用）|
| **refId** | A / B / C ...，多查询时引用用 |
| **format** | Time series / Table / Heatmap |
| **instant** | 仅查"现在"一个值（用于 Stat / Gauge） |
| **range** | 查时间范围（默认） |

**`$__interval` 是什么**：Grafana 根据当前时间范围 + 图表宽度自动算出"采样步长"，传给数据源。这样缩放时间窗口时不会查出几万个数据点把浏览器卡死。

## 四、Field options & Overrides

每个 Panel 的右侧面板（在 Panel 编辑器里）：

| 区域 | 控制 |
|------|------|
| **Standard options** | unit（单位）、min/max、decimals、color scheme |
| **Thresholds** | 阈值色块（绿 / 黄 / 红） |
| **Value mappings** | 值映射（1 → "Up"，0 → "Down"） |
| **Data links** | 点击数据点跳转 URL（带变量） |
| **Overrides** | 针对特定 series 单独设样式（如 P99 线加粗） |

**单位（unit）必须设对**：选 `seconds (s)`、`bytes (IEC)`、`percent (0.0-1.0)` 等，UI 才会显示 `1.2 ms` / `512 KiB`，而不是干巴巴的数字。

## 五、变量（Variables）

变量是 Grafana 的灵魂——同一份 dashboard 通过下拉框切换实例 / 环境 / 服务。

### 5.1 类型

| 类型 | 取值来自 | 典型用法 |
|------|---------|---------|
| **Query** | 数据源查询 | `label_values(up, instance)` 拿所有实例 |
| **Custom** | 手填值列表 | 写死 `dev,staging,prod` |
| **Constant** | 固定值（隐藏） | dashboard 内部模板 |
| **Datasource** | 列出某类型所有数据源 | 多 Prometheus 切换 |
| **Interval** | 时间间隔 | `1m,5m,1h` 给查询 step |
| **Text box** | 自由输入 | 搜索 user_id |
| **Ad hoc filters** | 自动生成的标签过滤 | 临时过滤 |

### 5.2 定义示例

Dashboard Settings → Variables → New：

```
Name: instance
Type: Query
Datasource: Prometheus
Query: label_values(webook_http_requests_total, instance)
Refresh: On time range change
Multi-value: ✅
Include All option: ✅
```

### 5.3 在查询中引用

```promql
sum by (pattern) (
  rate(webook_http_requests_total{instance=~"$instance"}[5m])
)
```

`$instance` → Grafana 替换成下拉框选中的值。多选时变成正则 `(a|b|c)`。

### 5.4 Repeat（按变量动态生成 Panel/Row）

Panel 设置 → Repeat options → Repeat by variable: `instance`，会按变量每个值生成一个 Panel。

**慎用**：变量值多了（几十个实例）会瞬间生成几十个 panel，浏览器卡死。

## 六、Annotations（事件标注）

在时间轴上标注事件（部署、故障、变更），便于关联分析"为什么这时候开始慢"。

### 来源

- **手动**：Ctrl+Click 图表添加
- **来自数据源**：查询某个返回事件的源（比如查 MySQL 的 `deployments` 表）
- **来自 Loki**：日志中匹配 `deploy=true`
- **来自 Prometheus alerts**：告警触发自动标

### 配置

Dashboard Settings → Annotations → New：

```
Name: Deployments
Datasource: Loki
Query: {app="webook"} |= "deployment.start"
```

## 七、Dashboard 链接 & 数据链接

| 类型 | 用途 |
|------|------|
| **Dashboard links** | dashboard 顶部的快捷跳转条（关联面板互跳） |
| **Panel links** | panel 标题旁小图标，点了跳 URL |
| **Data links** | 点数据点（曲线某个值）跳带变量的 URL |

最常用：从 "总览大盘" 通过 data link 跳到 "实例详情大盘" 并带上 `instance` 变量。

## 八、Time range 与刷新

| 设置 | 说明 |
|------|------|
| **Time range** | 默认时间窗口（建议 last 1h） |
| **Auto refresh** | 浏览器自动刷新间隔（建议 30s，太短压数据源） |
| **Time picker - quick ranges** | 自定义快速选项（如 "上一次发布后" 用相对时间） |

**关键**：每个 panel 也可以单独设 time range（比如总览看 last 1h，但"过去 7 天 P99 趋势"那个 panel override 成 7d）。

## 九、Dashboard JSON 怎么编辑

UI 拖拽生成 JSON，但**生产做法是 Git 管理 JSON**：

1. UI 上调好
2. Settings → JSON Model → 复制
3. 粘贴到 `provisioning/dashboards/<name>.json`
4. PR review

### JSON 关键字段示例

```json
{
  "title": "Webook Overview",
  "uid": "webook-overview",          // 必填，URL 路径用，环境间一致
  "tags": ["webook", "prod"],
  "timezone": "browser",
  "schemaVersion": 39,
  "refresh": "30s",
  "time": { "from": "now-1h", "to": "now" },
  "templating": { "list": [...] },
  "panels": [
    {
      "id": 1,
      "title": "QPS",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(webook_http_requests_total[$__rate_interval]))",
          "legendFormat": "total",
          "refId": "A"
        }
      ],
      "gridPos": { "x": 0, "y": 0, "w": 12, "h": 8 },
      "fieldConfig": {
        "defaults": { "unit": "reqps" }
      }
    }
  ]
}
```

`gridPos` 是布局：宽度 24 列 grid，`w/h` 是宽高，`x/y` 是位置。

### 处理 datasource uid

直接 hardcode `"uid": "prometheus"` 在多环境（dev/prod 数据源 uid 不同）会断。两种解法：

**方法 A：固定 uid**：所有环境 provisioning 用相同 uid（推荐，最简单）。

**方法 B：用 `${DS_PROMETHEUS}` 占位**：导出时 Grafana 生成 `__inputs`，导入时让用户选；但 provisioning 不友好。

webook 推荐方法 A。

## 十、设计原则

### 一个 dashboard 表达一个故事

不要一个 dashboard 塞 50 个 panel。按场景拆：

```
webook-overview      （SRE 看的总览：HTTP / Go / DB / Cache 各 4-6 个核心指标）
webook-http          （Web 工程师看的：按路由 QPS / 延迟 / 错误，深入到每个接口）
webook-db            （DBA 看的：MySQL 连接、慢查询、复制延迟）
webook-business      （PM 看的：注册数、发布数、互动数）
```

### 黄金信号 / RED / USE

设计 panel 时套这三套方法之一：

- **黄金信号**（Google SRE）：Latency / Traffic / Errors / Saturation
- **RED**（针对服务）：Rate / Errors / Duration
- **USE**（针对资源）：Utilization / Saturation / Errors

不要凭感觉选指标。

### 视觉一致

- 单位统一（时间全 ms 或全 s，不要混）
- 颜色语义统一（绿 = 健康，红 = 错误，黄 = 警告）
- 阈值统一（错误率红线全部 1%）

## 十一、复用社区 dashboard

`grafana.com/grafana/dashboards/` 几千个免费模板。webook 已用：

| ID | 用途 |
|----|------|
| 6671 | Go Processes |
| 14031 | Gin Prometheus |
| 14057 | MySQL Overview |
| 11835 | Redis Dashboard |
| 7589 | Kafka Exporter |
| 1860 | Node Exporter Full |
| 3662 | Prometheus 2.0 Overview |

**导入方法**：
1. UI：Dashboards → New → Import → 填 ID → 选数据源
2. Provisioning：下载 JSON → 改 datasource uid → 丢进 dashboards 目录

**社区模板的坑**：
- 旧模板用了已废弃的 panel 类型（Singlestat → Stat），导入后样式怪
- 指标名字假设的是 Prometheus 标准，与你的指标名不一致要手动改
- 变量查询用了 `label_values(metric, label)` 你 metric 没采就空
