# 最佳实践与排查指南

## 指标设计原则

### 命名规范

```
{namespace}_{subsystem}_{name}_{unit}
```

| 部分 | 规则 | 示例 |
|------|------|------|
| namespace | 应用名 | `webook` |
| subsystem | 模块 | `http`、`db`、`cache` |
| name | 描述 | `requests`、`connections` |
| unit | 单位后缀 | `_total`(Counter)、`_seconds`(时间)、`_bytes`(大小) |

**后缀约定**：
- Counter 必须以 `_total` 结尾
- 时间用秒（`_seconds`），不用毫秒
- 大小用字节（`_bytes`），不用 KB/MB
- 信息型指标用 `_info` 结尾（`build_info{version="1.0"} 1`）

### 标签基数控制

标签的每种组合都是一条独立的时间序列。标签值的数量叫做**基数（cardinality）**。

```
# 好：用路由模板 pattern（基数 = 路由数 ≈ 20）
pattern="/article/:id"

# 坏：用实际 URL path（基数 = 文章数 ≈ 无限）
pattern="/article/12345"
```

**规则**：
- 单个指标的标签组合不要超过 1000
- 不要把 user_id、request_id、IP 这种高基数值放进标签
- 用 `ctx.FullPath()`（返回路由 pattern）而不是 `ctx.Request.URL.Path`（返回实际 URL）
- 标签名用 `pattern`，明确语义；webook 中间件已这样做
- 状态码用原始值（200/404/500），不要用自定义分类

### 选择指标类型

| 你想知道 | 用什么 |
|----------|--------|
| 发生了多少次 | Counter + `rate()` |
| 当前是多少 | Gauge |
| 分布情况（P99 等） | Histogram |
| 精确分位数（单实例） | Summary |

## Histogram 桶设置

### 默认桶

```go
[]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
```

对应场景：Web API，大部分请求 5ms~500ms，少量慢请求到 5s。

### 按场景调整

```go
// 数据库操作（1ms~10s）
[]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10}

// 缓存读取（0.1ms~100ms）
[]float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1}

// 文件上传（100ms~60s）
[]float64{0.1, 0.5, 1, 2, 5, 10, 30, 60}
```

**原则**：桶边界要覆盖你关心的范围，在热点区域密一些。桶太多浪费存储，太少精度差。10~15 个桶通常够用。

## PromQL 常见陷阱

### 1. Counter 直接比较

```promql
# 错：Counter 是累加值，直接比较没意义
webook_http_requests_total > 1000

# 对：用 rate() 看速率
rate(webook_http_requests_total[5m]) > 10
```

### 2. rate 窗口太小

```promql
# 错：窗口内可能只有 1 个样本（scrape_interval=15s）
rate(webook_http_requests_total[15s])

# 对：至少 4 × scrape_interval
rate(webook_http_requests_total[1m])

# 推荐：5 分钟窗口，平衡平滑度和灵敏度
rate(webook_http_requests_total[5m])
```

### 3. histogram_quantile 缺 le

```promql
# 错：聚合时丢掉了 le 标签
histogram_quantile(0.99, sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (pattern))

# 对：by 里必须包含 le
histogram_quantile(0.99, sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern))
```

### 4. 除法除以 0

```promql
# 可能除以 0（没有请求时 count=0）
rate(duration_sum[5m]) / rate(duration_count[5m])

# 安全写法：没数据时返回空而不是 NaN
rate(duration_sum[5m]) / (rate(duration_count[5m]) > 0)
```

### 5. sum 丢失标签

```promql
# sum 默认去掉所有标签
sum(rate(webook_http_requests_total[5m]))  # 只剩一个数字

# 保留需要的标签
sum(rate(webook_http_requests_total[5m])) by (pattern)
```

### 6. increase 返回浮点数

```promql
# increase 返回的是浮点数估算值，不是精确整数
increase(webook_http_requests_total[1h])  # 可能返回 99.7 而不是 100
```

## 性能排查清单

遇到性能问题时，按此顺序排查：

### Step 1: 确认现象

```promql
# 整体 QPS 是否异常
sum(rate(webook_http_requests_total[5m]))

# 错误率
sum(rate(webook_http_requests_total{status=~"5.."}[5m]))
  /
sum(rate(webook_http_requests_total[5m]))

# P99 延迟
histogram_quantile(0.99, rate(webook_http_requests_duration_seconds_bucket[5m]))
```

### Step 2: 定位接口

```promql
# 哪个接口最慢
topk(5, histogram_quantile(0.99,
  sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern)
))

# 哪个接口错误最多
topk(5, sum(rate(webook_http_requests_total{status=~"5.."}[5m])) by (pattern))
```

### Step 3: 检查资源

```promql
# goroutine 是否泄漏
go_goroutines

# 内存是否异常
go_memstats_alloc_bytes / 1024 / 1024

# GC 是否频繁
rate(go_gc_duration_seconds_count[5m])

# CPU 使用率
rate(process_cpu_seconds_total[5m]) * 100
```

### Step 4: 检查依赖

```promql
# MySQL 连接是否打满
mysql_global_status_threads_connected

# MySQL 慢查询
rate(mysql_global_status_slow_queries[5m])

# Redis 命中率是否下降
rate(redis_keyspace_hits_total[5m])
  /
(rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m]))

# Kafka 是否积压
sum(kafka_consumergroup_lag) by (consumergroup, topic)
```

### Step 5: 对比时间线

```promql
# 和 1 小时前对比
rate(webook_http_requests_total[5m])
  /
rate(webook_http_requests_total[5m] offset 1h)

# 和昨天对比
rate(webook_http_requests_total[5m])
  /
rate(webook_http_requests_total[5m] offset 1d)
```

## RED 方法

监控微服务的三个黄金信号：

| 信号 | 指标 | PromQL |
|------|------|--------|
| **R**ate（速率） | QPS | `sum(rate(webook_http_requests_total[5m]))` |
| **E**rrors（错误） | 错误率 | `sum(rate(...{status=~"5.."}[5m])) / sum(rate(...[5m]))` |
| **D**uration（延迟） | P99 | `histogram_quantile(0.99, ...)` |

## USE 方法

监控基础资源：

| 信号 | 含义 | 示例 |
|------|------|------|
| **U**tilization（利用率） | 资源忙碌程度 | CPU 使用率、内存占用率、连接池使用率 |
| **S**aturation（饱和度） | 排队程度 | goroutine 数、Kafka lag、线程池队列长度 |
| **E**rrors（错误） | 错误计数 | 5xx、连接失败、超时 |

## Prometheus 自身监控

```promql
# Prometheus 每秒抓取的样本数
rate(prometheus_tsdb_head_samples_appended_total[5m])

# 当前活跃时间序列数（基数）
prometheus_tsdb_head_series

# 抓取耗时
prometheus_target_interval_length_seconds{quantile="0.99"}

# 存储块数
prometheus_tsdb_blocks_loaded

# 磁盘占用预估
prometheus_tsdb_storage_blocks_bytes
```

如果 `prometheus_tsdb_head_series` 持续增长，说明有高基数标签在污染指标。
