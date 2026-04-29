# Grafana Dashboard

把 JSON 格式的 dashboard 文件放到本目录下会自动加载。

## 项目自有 Dashboard（已 provisioned）

| 文件 | UID | 用途 |
|------|-----|------|
| `webook-overview.json` | `webook-overview` | SRE 主大盘：HTTP/Go/DB/Cache 核心指标 |
| `webook-ops.json` | `webook-ops` | 运维深度：慢查询、GC、连接池、goroutine |
| `webook-tracing.json` | `webook-tracing` | Tracing 入口：HTTP 概览 + Zipkin Explore/UI 跳转指引 |
| `linux-host.json` | `linux-host` | 宿主机 CPU/内存/磁盘/网络 |

**Tracing 查看路径**（打开 `Webook / Tracing` 后按提示操作）：
1. Explore 方式：左侧切换 `Zipkin` 数据源 → Query Type: `Search` → Service `webook`
2. 直连 Zipkin UI：`http://<host>:9411`

## 推荐开源模板

启动后登录 Grafana（默认 admin/admin），手动导入以下 dashboard：

**导入方式**：Dashboards → New → Import → 填 ID → Load → 选 Prometheus 数据源。

### 应用层

| ID | 名称 | 用途 |
|----|------|------|
| 6671 | Go Processes | Go runtime（goroutine、GC、堆内存、CPU） |
| 10826 | Go Metrics | Go runtime（比 6671 更详细，含 GC 暂停分布） |
| 14031 | Gin Prometheus | Gin HTTP（QPS、延迟分位数、状态码分布） |

### 数据库

| ID | 名称 | 用途 |
|----|------|------|
| 14057 | MySQL Overview | MySQL 总览（连接数、QPS、慢查询） |
| 7362 | MySQL Overview (Percona) | MySQL 更全面（连接池、锁、复制） |
| 11835 | Redis Dashboard | Redis 总览（命中率、内存、连接数） |
| 763 | Redis Dashboard for Prometheus | Redis（另一风格，含 key 空间分布） |

### 消息队列

| ID | 名称 | 用途 |
|----|------|------|
| 7589 | Kafka Exporter Overview | Kafka 总览（消费 lag、吞吐） |
| 12460 | Kafka Exporter | Kafka（含 Topic 级别详情） |

### 基础设施

| ID | 名称 | 用途 |
|----|------|------|
| 1860 | Node Exporter Full | 宿主机 CPU/内存/磁盘/网络（需 node-exporter） |
| 3662 | Prometheus 2.0 Overview | Prometheus 自身健康（抓取耗时、序列数、存储） |

### 推荐优先级

```
必装：6671 + 14031          → Go + HTTP（应用核心）
推荐：1860 + 3662           → 宿主机 + Prometheus 自身
已有：14057 + 11835 + 7589  → MySQL + Redis + Kafka
```

## 自定义面板

1. Dashboards → New → New Dashboard → Add visualization
2. Data source 选 **Prometheus**
3. 在 Query 框写 PromQL：

| 查询 | 含义 | 推荐图表类型 |
|------|------|-------------|
| `rate(webook_http_requests_total[5m])` | QPS（按路径） | Time series |
| `sum(rate(webook_http_requests_total[5m]))` | 全局 QPS | Stat |
| `histogram_quantile(0.99, rate(webook_http_requests_duration_seconds_bucket[5m]))` | P99 响应时间 | Time series |
| `rate(webook_http_requests_duration_seconds_sum[5m]) / rate(webook_http_requests_duration_seconds_count[5m])` | 平均响应时间 | Time series |
| `webook_http_requests_in_flight` | 当前活跃请求 | Gauge |
| `go_goroutines` | goroutine 数 | Time series |
| `go_memstats_alloc_bytes / 1024 / 1024` | 堆内存 MB | Time series |
| `rate(process_cpu_seconds_total[5m]) * 100` | CPU 使用率 % | Time series |
| `sum(rate(webook_http_requests_total{status=~"5.."}[5m])) / sum(rate(webook_http_requests_total[5m])) * 100` | 错误率 % | Stat |

4. 右侧面板选图表类型（Time series / Stat / Gauge / Table）
5. 保存

## 常用操作

- **时间范围**：右上角选 Last 15 minutes / 1 hour / 6 hours
- **自动刷新**：右上角刷新图标旁设置 10s / 30s / 1m
- **变量筛选**：导入的面板自带 instance/job 下拉框，选 `webook-core` 或 `webook-chat`
- **全屏查看**：面板标题 → 三个点 → View
- **快速对比**：按住 Ctrl 点选多条曲线高亮对比
- **标注事件**：在图表上 Ctrl+Click 添加 Annotation（如标记发布时间）
