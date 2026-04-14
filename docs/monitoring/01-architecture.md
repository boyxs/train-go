# Prometheus 监控架构

## 整体架构

```
┌─────────────┐    scrape /metrics    ┌──────────────┐    query    ┌──────────┐
│  webook-app │◄──────────────────────│  Prometheus  │◄───────────│  Grafana │
│  :8089      │                       │  :9090       │            │  :3001   │
└─────────────┘                       └──────┬───────┘            └──────────┘
                                             │
                          ┌──────────────────┼──────────────────┐
                          │                  │                  │
                   ┌──────┴──────┐  ┌───────┴───────┐  ┌──────┴──────┐
                   │   mysqld    │  │    redis      │  │    kafka    │
                   │  exporter   │  │   exporter    │  │   exporter  │
                   │  :9104      │  │   :9121       │  │   :9308     │
                   └──────┬──────┘  └───────┬───────┘  └──────┬──────┘
                          │                 │                  │
                   ┌──────┴──────┐  ┌───────┴───────┐  ┌──────┴──────┐
                   │    MySQL    │  │     Redis     │  │    Kafka    │
                   │    :3306    │  │     :6379     │  │    :9092    │
                   └─────────────┘  └───────────────┘  └─────────────┘
```

## 核心概念

### Pull 模型

Prometheus 主动拉取（scrape）目标的 `/metrics` 端点，而不是目标推送数据。

- **优点**：目标无需知道 Prometheus 存在；Prometheus 挂了不影响应用；容易判断目标是否存活
- **缺点**：短生命周期的 Job（如批处理）需要用 Pushgateway 中转

### 时间序列

每条时间序列由 **指标名 + 标签集** 唯一标识：

```
webook_http_requests_total{method="GET", path="/article/:id", status="200"}
```

- `webook_http_requests_total`：指标名
- `{method="GET", path="/article/:id", status="200"}`：标签（label），用于多维度筛选

### 四种指标类型

| 类型 | 特点 | 典型用途 | 示例 |
|------|------|----------|------|
| **Counter** | 只增不减，重启归零 | 请求数、错误数、消息处理数 | `webook_http_requests_total` |
| **Gauge** | 可增可减 | 当前连接数、队列长度、温度 | `webook_http_requests_in_flight` |
| **Histogram** | 按桶分布 + 累计计数/总和 | 请求延迟、响应大小 | `webook_http_requests_duration_seconds` |
| **Summary** | 客户端计算分位数 | 请求延迟（精确分位数） | `webook_http_requests_duration_seconds_summary` |

### Histogram vs Summary

| 维度 | Histogram | Summary |
|------|-----------|---------|
| 分位数计算 | 服务端（PromQL `histogram_quantile`） | 客户端（应用内计算） |
| 可聚合 | 可以跨实例聚合 | 不能聚合（分位数不可加） |
| 精度 | 取决于桶边界，有误差 | 精确 |
| 配置 | 定义桶边界 `[]float64{0.01, 0.05, ...}` | 定义分位数 `{0.5: 0.05, 0.99: 0.001}` |
| 推荐场景 | 多实例、需要聚合 | 单实例、需要精确值 |

**实践建议**：优先用 Histogram，多实例部署时 Summary 无法聚合。

## webook 指标清单

### 应用指标（中间件自动采集）

| 指标名 | 类型 | 标签 | 含义 |
|--------|------|------|------|
| `webook_http_requests_total` | Counter | method, path, status | 请求总数 |
| `webook_http_requests_duration_seconds` | Histogram | method, path | 响应时间分布 |
| `webook_http_requests_duration_seconds_summary` | Summary | method, path | 响应时间分位数 |
| `webook_http_requests_in_flight` | Gauge | method, path | 当前处理中请求数 |

### Go 运行时指标（自动暴露）

| 指标名 | 类型 | 含义 |
|--------|------|------|
| `go_goroutines` | Gauge | 当前 goroutine 数 |
| `go_gc_duration_seconds` | Summary | GC 耗时 |
| `go_memstats_alloc_bytes` | Gauge | 当前堆内存分配 |
| `go_memstats_sys_bytes` | Gauge | 从 OS 获取的总内存 |
| `go_threads` | Gauge | OS 线程数 |
| `process_cpu_seconds_total` | Counter | CPU 使用时间 |
| `process_resident_memory_bytes` | Gauge | 常驻内存 |
| `process_open_fds` | Gauge | 打开的文件描述符数 |

### Exporter 指标

| 来源 | 关键指标 |
|------|----------|
| MySQL | `mysql_global_status_threads_connected`、`mysql_global_status_queries`、`mysql_global_status_slow_queries` |
| Redis | `redis_connected_clients`、`redis_memory_used_bytes`、`redis_keyspace_hits_total`、`redis_keyspace_misses_total` |
| Kafka | `kafka_consumergroup_lag`、`kafka_topic_partition_current_offset`、`kafka_brokers` |
