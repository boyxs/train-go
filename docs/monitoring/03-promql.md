# PromQL 完全指南

## 基础语法

### 选择器

```promql
# 指标名直接查询（瞬时向量）
webook_http_requests_total

# 标签过滤
webook_http_requests_total{method="GET"}
webook_http_requests_total{status="200", pattern="/hello"}

# 标签匹配操作符
webook_http_requests_total{method="GET"}         # 精确匹配
webook_http_requests_total{method!="GET"}        # 不等于
webook_http_requests_total{pattern=~"/article.*"} # 正则匹配
webook_http_requests_total{pattern!~"/article.*"} # 正则不匹配

# 范围向量（最近 5 分钟的所有样本）
webook_http_requests_total[5m]

# 偏移（1 小时前的值）
webook_http_requests_total offset 1h
```

### 时间单位

| 单位 | 含义 | 示例 |
|------|------|------|
| `s` | 秒 | `[30s]` |
| `m` | 分 | `[5m]` |
| `h` | 时 | `[1h]` |
| `d` | 天 | `[7d]` |
| `w` | 周 | `[2w]` |
| `y` | 年 | `[1y]` |

## 函数详解

### rate() — Counter 的速率

Counter 是累加值，直接看没意义，必须用 `rate()` 算每秒增长率：

```promql
# 每秒请求数（QPS），过去 5 分钟的平均
rate(webook_http_requests_total[5m])

# 按路径分组的 QPS
rate(webook_http_requests_total[5m])

# 每秒错误数
rate(webook_http_requests_total{status=~"5.."}[5m])
```

**注意**：
- `rate()` 只能用于 Counter，自动处理重启归零
- 范围窗口至少要包含 2 个样本点（`scrape_interval=15s` 时，最小 `[30s]`）
- 推荐窗口 = 4 × scrape_interval = `[1m]`（15s 间隔时）

### irate() — 瞬时速率

```promql
# 基于最近两个样本的瞬时速率，更敏感但更抖
irate(webook_http_requests_total[5m])
```

| | rate() | irate() |
|---|--------|---------|
| 计算方式 | 窗口内平均 | 最近两个点 |
| 平滑度 | 平滑 | 抖动大 |
| 适用 | 告警、SLO | 临时排查看峰值 |

### increase() — 增长总量

```promql
# 过去 1 小时总请求数
increase(webook_http_requests_total[1h])

# 过去 24 小时错误总数
increase(webook_http_requests_total{status=~"5.."}[24h])
```

`increase(x[5m])` ≈ `rate(x[5m]) * 300`

### histogram_quantile() — 分位数计算

```promql
# P99 响应时间
histogram_quantile(0.99, rate(webook_http_requests_duration_seconds_bucket[5m]))

# P95
histogram_quantile(0.95, rate(webook_http_requests_duration_seconds_bucket[5m]))

# P50（中位数）
histogram_quantile(0.50, rate(webook_http_requests_duration_seconds_bucket[5m]))

# 按路径分组的 P99
histogram_quantile(0.99, sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le, pattern))
```

**重要**：
- 必须对 `_bucket` 后缀的指标使用
- 必须保留 `le` 标签（桶边界）
- `rate()` 在 `histogram_quantile()` 里面
- 分位数在桶边界之间做线性插值，精度取决于桶边界设置

### 聚合操作符

```promql
# 总和
sum(rate(webook_http_requests_total[5m]))

# 按标签分组
sum(rate(webook_http_requests_total[5m])) by (pattern)
sum(rate(webook_http_requests_total[5m])) by (method, status)

# 排除标签分组
sum(rate(webook_http_requests_total[5m])) without (instance, job)

# 其他聚合函数
avg(go_goroutines)                    # 平均
max(go_memstats_alloc_bytes)          # 最大
min(process_cpu_seconds_total)         # 最小
count(up)                              # 计数
topk(5, rate(webook_http_requests_total[5m]))  # Top 5
bottomk(3, go_goroutines)             # Bottom 3
```

### 数学运算

```promql
# 错误率（百分比）
sum(rate(webook_http_requests_total{status=~"5.."}[5m]))
  /
sum(rate(webook_http_requests_total[5m]))
  * 100

# 平均响应时间
rate(webook_http_requests_duration_seconds_sum[5m])
  /
rate(webook_http_requests_duration_seconds_count[5m])

# 内存使用率（MB）
go_memstats_alloc_bytes / 1024 / 1024

# Redis 缓存命中率
redis_keyspace_hits_total
  /
(redis_keyspace_hits_total + redis_keyspace_misses_total)
  * 100
```

### 常用函数速查

| 函数 | 输入 | 输出 | 用途 |
|------|------|------|------|
| `rate(v[t])` | Counter 范围向量 | 瞬时向量 | 每秒增长率 |
| `irate(v[t])` | Counter 范围向量 | 瞬时向量 | 瞬时增长率 |
| `increase(v[t])` | Counter 范围向量 | 瞬时向量 | 时间窗口内增长量 |
| `histogram_quantile(φ, v)` | Histogram bucket | 瞬时向量 | 分位数 |
| `sum(v)` | 瞬时向量 | 瞬时向量 | 求和 |
| `avg(v)` | 瞬时向量 | 瞬时向量 | 平均 |
| `max(v) / min(v)` | 瞬时向量 | 瞬时向量 | 最大/最小 |
| `topk(k, v)` | 瞬时向量 | 瞬时向量 | 前 k 个 |
| `abs(v)` | 瞬时向量 | 瞬时向量 | 绝对值 |
| `ceil(v) / floor(v)` | 瞬时向量 | 瞬时向量 | 向上/下取整 |
| `round(v)` | 瞬时向量 | 瞬时向量 | 四舍五入 |
| `delta(v[t])` | Gauge 范围向量 | 瞬时向量 | 差值 |
| `deriv(v[t])` | Gauge 范围向量 | 瞬时向量 | 导数（变化趋势） |
| `predict_linear(v[t], s)` | Gauge 范围向量 | 瞬时向量 | 线性预测 s 秒后的值 |
| `changes(v[t])` | 范围向量 | 瞬时向量 | 值变化次数 |
| `resets(v[t])` | Counter 范围向量 | 瞬时向量 | 重置次数（重启） |
| `absent(v)` | 瞬时向量 | 瞬时向量 | 指标不存在时返回 1 |
| `label_replace(v, ...)` | 瞬时向量 | 瞬时向量 | 修改标签 |
| `sort(v) / sort_desc(v)` | 瞬时向量 | 瞬时向量 | 排序 |
| `time()` | 无 | 标量 | 当前 Unix 时间戳 |
| `vector(s)` | 标量 | 瞬时向量 | 标量转向量 |

## 二元操作符

### 算术操作符

`+`、`-`、`*`、`/`、`%`（取模）、`^`（幂）

```promql
# 可用内存
go_memstats_sys_bytes - go_memstats_alloc_bytes

# 每个请求的平均字节数
rate(http_response_size_bytes_sum[5m]) / rate(http_response_size_bytes_count[5m])
```

### 比较操作符

`==`、`!=`、`>`、`<`、`>=`、`<=`

```promql
# 响应时间超过 1 秒的路径
histogram_quantile(0.99, rate(webook_http_requests_duration_seconds_bucket[5m])) > 1

# goroutine 数大于 1000
go_goroutines > 1000

# 加 bool 修饰符返回 0/1 而不是过滤
go_goroutines > bool 1000
```

### 逻辑操作符

`and`、`or`、`unless`

```promql
# 同时满足：QPS > 100 且 错误率 > 5%
rate(webook_http_requests_total[5m]) > 100
  and
rate(webook_http_requests_total{status=~"5.."}[5m]) > 0.05

# 排除某些路径
rate(webook_http_requests_total[5m]) unless rate(webook_http_requests_total{pattern="/metrics"}[5m])
```

### 向量匹配

```promql
# 一对一匹配（标签完全相同）
metric_a / metric_b

# 指定匹配标签
metric_a / on(method, pattern) metric_b

# 忽略某些标签匹配
metric_a / ignoring(status) metric_b

# 多对一匹配
metric_a / on(pattern) group_left metric_b
```
