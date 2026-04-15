# webook 实战查询手册

> 所有查询基于 webook 的四个自定义指标 + Go 运行时 + Exporter 指标。
> 单位：延迟类返回**秒**，Grafana 设 `unit: s` 会自动转 ms 显示（如 0.091s → 91ms）。

## HTTP 流量分析

### QPS（每秒请求数）

```promql
# 全局 QPS（最常用，一个数字看全局负载）
sum(rate(webook_http_requests_total[5m]))

# 按路径 QPS（看哪个接口最忙）
sum(rate(webook_http_requests_total[5m])) by (pattern)

# 按方法 + 路径（区分 GET 和 POST 同路径）
sum(rate(webook_http_requests_total[5m])) by (method, pattern)

# 单个接口 QPS（排查特定接口）
rate(webook_http_requests_total{pattern="/user/login"}[5m])

# Top 10 高流量接口（找热点）
topk(10, sum(rate(webook_http_requests_total[5m])) by (pattern))

# 过去 1 小时总请求数（看绝对量）
sum(increase(webook_http_requests_total[1h]))

# 按路径的过去 1 小时请求数
sum(increase(webook_http_requests_total[1h])) by (pattern)

# QPS 环比：当前 vs 1 小时前（> 1 表示在增长）
sum(rate(webook_http_requests_total[5m]))
  /
sum(rate(webook_http_requests_total[5m] offset 1h))

# QPS 环比：当前 vs 昨天同一时刻
sum(rate(webook_http_requests_total[5m]))
  /
sum(rate(webook_http_requests_total[5m] offset 1d))

# QPS 突增检测：当前是过去 1 小时均值的 3 倍以上
sum(rate(webook_http_requests_total[5m]))
  >
3 * sum(rate(webook_http_requests_total[1h]))
```

### 错误率

```promql
# 全局 5xx 错误率（百分比，最重要的 RED 指标之一）
sum(rate(webook_http_requests_total{status=~"5.."}[5m]))
  /
sum(rate(webook_http_requests_total[5m]))
  * 100

# 按路径的 5xx 错误率（定位哪个接口出问题）
sum(rate(webook_http_requests_total{status=~"5.."}[5m])) by (pattern)
  /
sum(rate(webook_http_requests_total[5m])) by (pattern)
  * 100

# 4xx 客户端错误率（按路径）
sum(rate(webook_http_requests_total{status=~"4.."}[5m])) by (pattern)
  /
sum(rate(webook_http_requests_total[5m])) by (pattern)
  * 100

# 非 200 请求占比
1 - sum(rate(webook_http_requests_total{status="200"}[5m]))
      /
    sum(rate(webook_http_requests_total[5m]))

# 5xx 错误绝对数（过去 1 小时）
sum(increase(webook_http_requests_total{status=~"5.."}[1h]))

# 5xx 错误绝对数按路径（过去 1 小时）
sum(increase(webook_http_requests_total{status=~"5.."}[1h])) by (pattern)

# 有 5xx 错误的接口列表（排查用，只看 > 0 的）
sum(rate(webook_http_requests_total{status=~"5.."}[5m])) by (pattern, status) > 0

# 有 4xx 错误的接口列表
sum(rate(webook_http_requests_total{status=~"4.."}[5m])) by (pattern, status) > 0

# 错误率安全除法（避免无请求时除以 0 返回 NaN）
sum(rate(webook_http_requests_total{status=~"5.."}[5m]))
  /
(sum(rate(webook_http_requests_total[5m])) > 0)
  * 100
```

### 状态码分布

```promql
# 各状态码 QPS 分布（看整体健康度）
sum(rate(webook_http_requests_total[5m])) by (status)

# 过去 1 小时各状态码总数
sum(increase(webook_http_requests_total[1h])) by (status)

# 按路径 + 状态码（最细粒度）
sum(rate(webook_http_requests_total[5m])) by (pattern, status)

# 成功率（200 占比）
sum(rate(webook_http_requests_total{status="200"}[5m]))
  /
sum(rate(webook_http_requests_total[5m]))
  * 100
```

## 延迟分析

### 分位数（Histogram）

```promql
# P50（中位数）— 大多数用户的体验
histogram_quantile(0.50, rate(webook_http_requests_duration_seconds_bucket[5m]))

# P90 — 90% 用户在此时间内完成
histogram_quantile(0.90, rate(webook_http_requests_duration_seconds_bucket[5m]))

# P95 — SLO 常用指标
histogram_quantile(0.95, rate(webook_http_requests_duration_seconds_bucket[5m]))

# P99 — 尾部延迟，最敏感的性能指标
histogram_quantile(0.99, rate(webook_http_requests_duration_seconds_bucket[5m]))

# 全局 P99（聚合所有路径）
histogram_quantile(0.99,
  sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le)
)

# 按路径的 P99（定位慢接口）
histogram_quantile(0.99,
  sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern)
)

# 按路径的 P50（对比中位数和 P99 看离散度）
histogram_quantile(0.50,
  sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern)
)

# 最慢的 5 个接口（P99 排名）
topk(5,
  histogram_quantile(0.99,
    sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern)
  )
)

# 最慢的 10 个接口（P99 排名，表格用）
topk(10,
  histogram_quantile(0.99,
    sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern)
  )
)

# P99 环比：当前 vs 1 小时前
histogram_quantile(0.99, sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le))
  /
histogram_quantile(0.99, sum(rate(webook_http_requests_duration_seconds_bucket[5m] offset 1h)) by (le))
```

### 平均响应时间

```promql
# 全局平均响应时间
rate(webook_http_requests_duration_seconds_sum[5m])
  /
rate(webook_http_requests_duration_seconds_count[5m])

# 按路径平均响应时间
sum(rate(webook_http_requests_duration_seconds_sum[5m])) by (pattern)
  /
sum(rate(webook_http_requests_duration_seconds_count[5m])) by (pattern)

# 安全除法（避免无请求时 NaN）
sum(rate(webook_http_requests_duration_seconds_sum[5m])) by (pattern)
  /
(sum(rate(webook_http_requests_duration_seconds_count[5m])) by (pattern) > 0)
```

### 分位数（Summary）

```promql
# Summary 直接给分位数值，不需要 histogram_quantile
# 精确但不可聚合，只适合单实例
webook_http_requests_duration_seconds_summary{quantile="0.99"}
webook_http_requests_duration_seconds_summary{quantile="0.95"}
webook_http_requests_duration_seconds_summary{quantile="0.9"}
webook_http_requests_duration_seconds_summary{quantile="0.5"}

# 按路径的 Summary P99
webook_http_requests_duration_seconds_summary{quantile="0.99"}
```

### 延迟桶分布

```promql
# 小于 10ms 的请求占比（快请求比例）
sum(rate(webook_http_requests_duration_seconds_bucket{le="0.01"}[5m]))
  /
sum(rate(webook_http_requests_duration_seconds_count[5m]))
  * 100

# 小于 100ms 的请求占比
sum(rate(webook_http_requests_duration_seconds_bucket{le="0.1"}[5m]))
  /
sum(rate(webook_http_requests_duration_seconds_count[5m]))
  * 100

# 小于 500ms 的请求占比
sum(rate(webook_http_requests_duration_seconds_bucket{le="0.5"}[5m]))
  /
sum(rate(webook_http_requests_duration_seconds_count[5m]))
  * 100

# 超过 1 秒的慢请求占比
1 - sum(rate(webook_http_requests_duration_seconds_bucket{le="1"}[5m]))
      /
    sum(rate(webook_http_requests_duration_seconds_count[5m]))

# 超过 5 秒的超慢请求占比
1 - sum(rate(webook_http_requests_duration_seconds_bucket{le="5"}[5m]))
      /
    sum(rate(webook_http_requests_duration_seconds_count[5m]))

# 各桶的请求分布（看延迟集中在哪个区间）
sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le)

# 按路径看各桶分布
sum(rate(webook_http_requests_duration_seconds_bucket{pattern="/article/reader/detail"}[5m])) by (le)

# Apdex 近似值（满意 < 100ms，容忍 < 500ms）
(
  sum(rate(webook_http_requests_duration_seconds_bucket{le="0.1"}[5m]))
  +
  sum(rate(webook_http_requests_duration_seconds_bucket{le="0.5"}[5m]))
)
  /
(2 * sum(rate(webook_http_requests_duration_seconds_count[5m])))
```

## 并发与负载

```promql
# 当前活跃请求数（全局）
sum(webook_http_requests_in_flight)

# 按路径的活跃请求
sum(webook_http_requests_in_flight) by (pattern)

# 按方法的活跃请求
sum(webook_http_requests_in_flight) by (method)

# 活跃请求最多的 5 个接口
topk(5, sum(webook_http_requests_in_flight) by (pattern))

# 活跃请求历史峰值（过去 1 小时）
max_over_time(sum(webook_http_requests_in_flight)[1h:])

# 并发突增检测
sum(webook_http_requests_in_flight) > 50
```

## Go 运行时

### Goroutine

```promql
# 当前 goroutine 数
go_goroutines{job="webook-app"}

# goroutine 变化趋势（正值=在增长，负值=在减少）
deriv(go_goroutines{job="webook-app"}[15m])

# goroutine 数量突增（1 小时内变化超过 100）
delta(go_goroutines{job="webook-app"}[1h]) > 100

# goroutine 过去 1 小时最大值
max_over_time(go_goroutines{job="webook-app"}[1h])

# goroutine 过去 1 小时最小值
min_over_time(go_goroutines{job="webook-app"}[1h])

# goroutine 泄漏检测：持续增长且不回落
# 如果 1 小时内一直在涨（deriv > 0），可能泄漏
deriv(go_goroutines{job="webook-app"}[1h]) > 0

# OS 线程数
go_threads{job="webook-app"}
```

### 内存

```promql
# 当前堆内存（MB）— 正在使用的
go_memstats_alloc_bytes{job="webook-app"} / 1024 / 1024

# 从 OS 获取的总内存（MB）— 进程占用的
go_memstats_sys_bytes{job="webook-app"} / 1024 / 1024

# 堆对象数（对象多 = GC 压力大）
go_memstats_heap_objects{job="webook-app"}

# 内存分配速率（bytes/s，越高 GC 越忙）
rate(go_memstats_alloc_bytes_total{job="webook-app"}[5m])

# 内存分配速率（MB/s，更直观）
rate(go_memstats_alloc_bytes_total{job="webook-app"}[5m]) / 1024 / 1024

# 进程常驻内存 RSS（MB，操作系统视角）
process_resident_memory_bytes{job="webook-app"} / 1024 / 1024

# 进程虚拟内存（MB）
process_virtual_memory_bytes{job="webook-app"} / 1024 / 1024

# 内存使用率 = 堆内存 / OS 分配
go_memstats_alloc_bytes{job="webook-app"}
  /
go_memstats_sys_bytes{job="webook-app"}
  * 100

# 栈内存（MB）
go_memstats_stack_inuse_bytes{job="webook-app"} / 1024 / 1024

# 内存泄漏检测：当前堆 + 1h 后线性预测
go_memstats_alloc_bytes{job="webook-app"} / 1024 / 1024
predict_linear(go_memstats_alloc_bytes{job="webook-app"}[1h], 3600) / 1024 / 1024

# 内存泄漏检测：4h 后预测超过 500MB 告警
predict_linear(go_memstats_alloc_bytes{job="webook-app"}[1h], 4 * 3600) / 1024 / 1024 > 500

# 内存过去 1 小时最高水位
max_over_time(go_memstats_alloc_bytes{job="webook-app"}[1h]) / 1024 / 1024
```

### GC

```promql
# GC 频率（每秒 GC 次数，正常值一般 < 10）
rate(go_gc_duration_seconds_count{job="webook-app"}[5m])

# GC P50 暂停耗时（大多数 GC 暂停多久）
go_gc_duration_seconds{job="webook-app", quantile="0.5"}

# GC P75 暂停耗时
go_gc_duration_seconds{job="webook-app", quantile="0.75"}

# GC P99 暂停耗时（尾部 GC 暂停）
go_gc_duration_seconds{job="webook-app", quantile="0.99"}

# GC 总暂停时间占比（> 0.05 即 5% 时间在 GC，需要优化）
rate(go_gc_duration_seconds_sum{job="webook-app"}[5m])

# GC 次数（过去 1 小时总共多少次）
increase(go_gc_duration_seconds_count{job="webook-app"}[1h])

# 每次 GC 平均回收的内存（bytes）
rate(go_memstats_alloc_bytes_total{job="webook-app"}[5m])
  /
rate(go_gc_duration_seconds_count{job="webook-app"}[5m])
```

### CPU & 进程

```promql
# CPU 使用率（百分比，单核 100% = 满载一个核）
rate(process_cpu_seconds_total{job="webook-app"}[5m]) * 100

# CPU 使用率过去 1 小时峰值
max_over_time(rate(process_cpu_seconds_total{job="webook-app"}[5m])[1h:]) * 100

# 打开的文件描述符
process_open_fds{job="webook-app"}

# 文件描述符上限
process_max_fds{job="webook-app"}

# 文件描述符使用率
process_open_fds{job="webook-app"} / process_max_fds{job="webook-app"} * 100

# 进程运行时间（秒，Grafana 设 unit: s 会自动显示为 xd xh xm）
time() - process_start_time_seconds{job="webook-app"}

# 进程重启次数（过去 1 小时，> 0 说明重启过）
resets(process_cpu_seconds_total{job="webook-app"}[1h])

# 进程启动时间戳（Unix 时间）
process_start_time_seconds{job="webook-app"}
```

## MySQL 关键查询

### 连接

```promql
# 当前连接数
mysql_global_status_threads_connected

# 正在运行的线程
mysql_global_status_threads_running

# 最大连接数
mysql_global_variables_max_connections

# 连接使用率（> 80% 需要关注）
mysql_global_status_threads_connected / mysql_global_variables_max_connections * 100

# 拒绝的连接数（连接池满了）
rate(mysql_global_status_aborted_connects[5m])

# 连接创建速率
rate(mysql_global_status_connections[5m])
```

### 查询

```promql
# QPS（总查询速率）
rate(mysql_global_status_queries[5m])

# 慢查询速率（> 0 需要关注）
rate(mysql_global_status_slow_queries[5m])

# 慢查询过去 1 小时总数
increase(mysql_global_status_slow_queries[1h])

# 各类语句速率
rate(mysql_global_status_commands_total{command="select"}[5m])
rate(mysql_global_status_commands_total{command="insert"}[5m])
rate(mysql_global_status_commands_total{command="update"}[5m])
rate(mysql_global_status_commands_total{command="delete"}[5m])

# 读写比
rate(mysql_global_status_commands_total{command="select"}[5m])
  /
(
  rate(mysql_global_status_commands_total{command="insert"}[5m])
  + rate(mysql_global_status_commands_total{command="update"}[5m])
  + rate(mysql_global_status_commands_total{command="delete"}[5m])
)
```

### InnoDB

```promql
# InnoDB 行操作速率
rate(mysql_global_status_innodb_row_ops_total[5m])

# InnoDB 缓冲池命中率（< 99% 需要加内存）
rate(mysql_global_status_innodb_buffer_pool_reads[5m])
  /
rate(mysql_global_status_innodb_buffer_pool_read_requests[5m])

# InnoDB 缓冲池使用量
mysql_global_status_innodb_buffer_pool_pages_data
  /
mysql_global_status_innodb_buffer_pool_pages_total
  * 100

# InnoDB 行锁等待时间
rate(mysql_global_status_innodb_row_lock_time[5m])

# InnoDB 行锁等待次数
rate(mysql_global_status_innodb_row_lock_waits[5m])

# InnoDB 死锁检测（有则 > 0）
rate(mysql_global_status_innodb_deadlocks[5m])
```

### 复制 & 存储

```promql
# 表打开缓存命中率
rate(mysql_global_status_table_open_cache_hits[5m])
  /
(rate(mysql_global_status_table_open_cache_hits[5m]) + rate(mysql_global_status_table_open_cache_misses[5m]))
  * 100

# 临时表创建到磁盘（多则需要调 tmp_table_size）
rate(mysql_global_status_created_tmp_disk_tables[5m])

# 网络流量
rate(mysql_global_status_bytes_received[5m])
rate(mysql_global_status_bytes_sent[5m])
```

## Redis 关键查询

### 命中率 & 性能

```promql
# 命中率（最重要的 Redis 指标，< 90% 需要关注）
rate(redis_keyspace_hits_total[5m])
  /
(rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m]))
  * 100

# 命中率安全除法
rate(redis_keyspace_hits_total[5m])
  /
((rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m])) > 0)
  * 100

# 每秒命令数
rate(redis_commands_total[5m])

# 按命令类型的每秒操作
rate(redis_commands_total[5m])

# 每秒过期 key 数
rate(redis_expired_keys_total[5m])

# 每秒淘汰 key 数（> 0 说明内存不够用了 maxmemory-policy 在生效）
rate(redis_evicted_keys_total[5m])

# 命令延迟（微秒）
rate(redis_commands_duration_seconds_total[5m]) / rate(redis_commands_total[5m])
```

### 内存

```promql
# 内存使用（MB）
redis_memory_used_bytes / 1024 / 1024

# 内存碎片率（> 1.5 说明碎片严重，< 1 说明在用 swap）
redis_memory_used_bytes / redis_memory_used_rss_bytes

# 内存使用趋势（过去 1 小时）
max_over_time(redis_memory_used_bytes[1h]) / 1024 / 1024

# 4h 后内存预测
predict_linear(redis_memory_used_bytes[1h], 4 * 3600) / 1024 / 1024
```

### 连接

```promql
# 当前客户端连接数
redis_connected_clients

# 阻塞客户端
redis_blocked_clients

# 被拒绝的连接
rate(redis_rejected_connections_total[5m])

# key 总数
redis_db_keys
```

## Kafka 关键查询

### Consumer Lag（最重要）

```promql
# 消费者 lag 总量
sum(kafka_consumergroup_lag)

# 按 topic + group 的 lag（定位哪个消费组积压）
sum(kafka_consumergroup_lag) by (consumergroup, topic)

# 按 partition 的 lag（定位哪个分区积压）
kafka_consumergroup_lag

# lag 增长趋势（正值 = 在积压，负值 = 在追赶）
deriv(sum(kafka_consumergroup_lag) by (consumergroup, topic)[15m])

# lag 超过阈值
sum(kafka_consumergroup_lag) by (consumergroup, topic) > 1000

# 4h 后 lag 预测
predict_linear(sum(kafka_consumergroup_lag) by (consumergroup, topic)[1h], 4 * 3600)
```

### 吞吐

```promql
# 每秒消息产出速率（Topic 级别）
sum(rate(kafka_topic_partition_current_offset[5m])) by (topic)

# 每秒消息消费速率
sum(rate(kafka_consumergroup_current_offset[5m])) by (consumergroup, topic)

# 产出 - 消费 = 积压速率（正值说明在积压）
sum(rate(kafka_topic_partition_current_offset[5m])) by (topic)
  -
sum(rate(kafka_consumergroup_current_offset[5m])) by (topic)
```

### 集群健康

```promql
# Broker 存活数（单节点应该 = 1）
kafka_brokers

# 分区数
count(kafka_topic_partition_current_offset) by (topic)

# 分区 Leader 数
count(kafka_topic_partition_leader) by (topic)
```

## Targets 健康检查

```promql
# 所有抓取目标状态（1=UP, 0=DOWN）
up

# 只看 DOWN 的目标
up == 0

# 各 job 抓取耗时（秒）
scrape_duration_seconds

# 抓取超时检测（超过 5 秒）
scrape_duration_seconds > 5

# 各 job 每次抓取的样本数
scrape_samples_scraped

# Prometheus 自身时间序列数（基数监控，持续增长说明有高基数标签）
prometheus_tsdb_head_series

# Prometheus 每秒入库样本数
rate(prometheus_tsdb_head_samples_appended_total[5m])

# Prometheus 存储块数
prometheus_tsdb_blocks_loaded

# Prometheus 磁盘占用（bytes）
prometheus_tsdb_storage_blocks_bytes

# Prometheus WAL 损坏次数（> 0 有数据风险）
prometheus_tsdb_wal_corruptions_total

# 查询耗时 P99
prometheus_engine_query_duration_seconds{quantile="0.99"}
```

## Grafana 面板对应查询

两个自动加载的面板（`webook-overview.json` / `webook-ops.json`）使用的查询汇总。

### Overview 面板

| 面板 | PromQL | 图表类型 |
|------|--------|---------|
| 全局 QPS | `sum(rate(webook_http_requests_total[5m]))` | Stat |
| 错误率 | `sum(rate(...{status=~"5.."}[5m])) / sum(rate(...[5m])) * 100` | Stat |
| P99 延迟 | `histogram_quantile(0.99, sum(rate(..._bucket[5m])) by (le))` | Stat |
| 活跃请求 | `sum(webook_http_requests_in_flight)` | Stat |
| Goroutines | `go_goroutines{job="webook-app"}` | Stat |
| 堆内存 | `go_memstats_alloc_bytes{job="webook-app"}` | Stat |
| QPS 按路径 | `sum(rate(webook_http_requests_total[5m])) by (pattern)` | Time series |
| 分位数曲线 | P50/P90/P95/P99 四条 `histogram_quantile` | Time series |
| 状态码分布 | `sum(rate(...[5m])) by (status)` | Stacked bar |
| P99 按路径 | `histogram_quantile(0.99, ... by (le, pattern))` | Time series |
| 平均延迟按路径 | `sum(rate(..._sum[5m])) by (pattern) / sum(rate(..._count[5m])) by (pattern)` | Time series |
| 活跃请求按路径 | `sum(webook_http_requests_in_flight) by (pattern)` | Time series |
| Goroutines 趋势 | `go_goroutines{job="webook-app"}` | Time series |
| 堆内存趋势 | `alloc_bytes` + `sys_bytes` 双线 | Time series |
| CPU 使用率 | `rate(process_cpu_seconds_total[5m]) * 100` | Time series |
| GC 频率 | `rate(go_gc_duration_seconds_count[5m])` | Time series |
| GC 暂停时间 | P50 + P99 双线 | Time series |
| 文件描述符 | `process_open_fds` | Time series |

### Ops 面板

| 面板 | PromQL | 图表类型 |
|------|--------|---------|
| 运行时间 | `time() - process_start_time_seconds` | Stat |
| 重启次数 | `resets(process_cpu_seconds_total[1h])` | Stat |
| Redis 命中率 | `rate(hits) / (rate(hits) + rate(misses)) * 100` | Gauge |
| MySQL 连接数 | `mysql_global_status_threads_connected` | Stat |
| Kafka Lag | `sum(kafka_consumergroup_lag)` | Stat |
| 内存分配率 | `rate(go_memstats_alloc_bytes_total[5m])` | Stat |
| Top 10 慢接口 | `topk(10, histogram_quantile(0.99, ... by (le, pattern)))` | Table |
| Top 10 高流量 | `topk(10, sum(rate(...[5m])) by (pattern))` | Table |
| 5xx 错误接口 | `sum(rate(...{status=~"5.."}[5m])) by (pattern, status) > 0` | Table |
| 4xx 错误接口 | `sum(rate(...{status=~"4.."}[5m])) by (pattern, status) > 0` | Table |
| Redis 命中率趋势 | 命中率公式 | Time series |
| MySQL QPS | `queries/s` + `slow queries/s` 双线 | Time series |
| Kafka Lag 趋势 | `sum(lag) by (consumergroup, topic)` | Time series |
| 内存泄漏检测 | 当前堆 + `predict_linear` 预测虚线 | Time series |
| Targets 状态 | `up`，值映射 1=UP(绿) 0=DOWN(红) | Table |

> **单位说明**：`histogram_quantile` 返回值单位是秒（和桶边界一致），Grafana 设了 `unit: s` 后会自动转为 ms/μs 显示（如 0.091s → 91ms）。这不是 bug，是 Grafana 的智能单位转换。

## 复合分析场景

### SLO 监控（99.9% 可用性）

```promql
# 过去 7 天的可用性（非 5xx 占比）
1 - (
  sum(increase(webook_http_requests_total{status=~"5.."}[7d]))
  /
  sum(increase(webook_http_requests_total[7d]))
)

# 过去 30 天剩余错误预算（0.1% = 允许的错误比例）
1 - (
  sum(increase(webook_http_requests_total{status=~"5.."}[30d]))
  /
  sum(increase(webook_http_requests_total[30d]))
) - 0.999

# 如果当前错误率持续，剩余预算还能撑多久（小时）
# 思路：剩余预算 / 当前消耗速率
(0.001 * sum(increase(webook_http_requests_total[30d]))
  - sum(increase(webook_http_requests_total{status=~"5.."}[30d])))
  /
sum(rate(webook_http_requests_total{status=~"5.."}[5m]))
  / 3600

# P99 延迟 SLO（99% 请求 < 500ms）
histogram_quantile(0.99,
  sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le)
) < 0.5
```

### 容量预估

```promql
# 基于过去 7 天趋势，预测 24 小时后的 goroutine 数
predict_linear(go_goroutines{job="webook-app"}[7d], 24 * 3600)

# 预测 24 小时后的内存使用（MB）
predict_linear(go_memstats_alloc_bytes{job="webook-app"}[7d], 24 * 3600) / 1024 / 1024

# 预测 24 小时后的 Kafka lag
predict_linear(sum(kafka_consumergroup_lag)[7d:], 24 * 3600)

# 预测 24 小时后的 MySQL 连接数
predict_linear(mysql_global_status_threads_connected[7d], 24 * 3600)

# 预测 24 小时后的 Redis 内存（MB）
predict_linear(redis_memory_used_bytes[7d], 24 * 3600) / 1024 / 1024

# QPS 增长趋势（过去 7 天的线性增长率）
deriv(sum(rate(webook_http_requests_total[1h]))[7d:1h])
```

### 关联分析

```promql
# 延迟高 + QPS 高的接口（可能是性能瓶颈）
histogram_quantile(0.99,
  sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern)
) > 0.5
and
sum(rate(webook_http_requests_total[5m])) by (pattern) > 1

# 高错误率 + 高流量（影响面最大的故障接口）
sum(rate(webook_http_requests_total{status=~"5.."}[5m])) by (pattern) > 0
and
sum(rate(webook_http_requests_total[5m])) by (pattern) > 10

# MySQL 慢查询增多的同时 HTTP 延迟上升（关联 DB 问题）
rate(mysql_global_status_slow_queries[5m]) > 0.1
and
histogram_quantile(0.99, sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le)) > 1

# Redis 命中率下降的同时 HTTP 延迟上升（关联缓存问题）
rate(redis_keyspace_hits_total[5m])
  /
(rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m]))
  < 0.9

# Kafka 积压的同时 goroutine 增长（消费者堆积）
sum(kafka_consumergroup_lag) > 500
and
go_goroutines{job="webook-app"} > 500

# 全链路健康快速检查（所有 > 0 表示异常）
(sum(rate(webook_http_requests_total{status=~"5.."}[5m])) or vector(0)) > 0  # HTTP 5xx
or
(rate(mysql_global_status_slow_queries[5m]) or vector(0)) > 0.1              # MySQL 慢查询
or
(sum(kafka_consumergroup_lag) or vector(0)) > 1000                           # Kafka 积压
```

### 排查速查表

遇到问题时按顺序查：

```promql
# Step 1: 确认现象 — 哪个指标异常
sum(rate(webook_http_requests_total[5m]))                           # QPS 正常？
sum(rate(webook_http_requests_total{status=~"5.."}[5m]))            # 有 5xx？
histogram_quantile(0.99, sum(rate(..._bucket[5m])) by (le))         # P99 正常？

# Step 2: 定位接口 — 哪个路径出问题
topk(5, sum(rate(webook_http_requests_total{status=~"5.."}[5m])) by (pattern))
topk(5, histogram_quantile(0.99, sum(rate(..._bucket[5m])) by (le, pattern)))

# Step 3: 检查应用 — Go 运行时有没有异常
go_goroutines{job="webook-app"}                                     # goroutine 泄漏？
go_memstats_alloc_bytes{job="webook-app"} / 1024 / 1024            # 内存正常？
rate(process_cpu_seconds_total{job="webook-app"}[5m]) * 100        # CPU 正常？

# Step 4: 检查依赖 — 下游服务有没有问题
mysql_global_status_threads_connected                               # MySQL 连接打满？
rate(mysql_global_status_slow_queries[5m])                          # MySQL 慢查询？
rate(redis_keyspace_hits_total[5m]) / (rate(hits[5m]) + rate(misses[5m]))  # Redis 命中率？
sum(kafka_consumergroup_lag) by (consumergroup, topic)              # Kafka 积压？

# Step 5: 对比历史 — 是突发还是渐变
sum(rate(webook_http_requests_total[5m])) / sum(rate(...[5m] offset 1h))   # vs 1 小时前
sum(rate(webook_http_requests_total[5m])) / sum(rate(...[5m] offset 1d))   # vs 昨天
```
