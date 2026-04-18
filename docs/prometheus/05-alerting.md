# 告警规则（参考）

> 当前 webook 未启用告警（B 档），本文档提供规则模板，后续需要时直接使用。

## 配置方式

### 1. 创建规则文件

```yaml
# prometheus/rules/webook.yml
groups:
  - name: webook
    rules:
      - alert: HighErrorRate
        expr: ...
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "..."
          description: "..."
```

### 2. 在 prometheus.yml 引入

```yaml
rule_files:
  - "rules/*.yml"
```

### 3. 在 docker-compose 挂载

```yaml
prometheus:
  volumes:
    - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
    - ./prometheus/rules:/etc/prometheus/rules   # 新增
```

## 告警规则模板

### HTTP 告警

```yaml
groups:
  - name: webook-http
    rules:
      # 5xx 错误率 > 5% 持续 5 分钟
      - alert: HighErrorRate
        expr: |
          sum(rate(webook_http_requests_total{status=~"5.."}[5m]))
          /
          sum(rate(webook_http_requests_total[5m]))
          > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "HTTP 5xx 错误率过高"
          description: "当前 5xx 错误率 {{ $value | humanizePercentage }}，超过 5% 阈值"

      # P99 延迟 > 2 秒持续 5 分钟
      - alert: HighLatency
        expr: |
          histogram_quantile(0.99,
            sum(rate(webook_http_requests_duration_seconds_bucket[5m])) by (le)
          ) > 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P99 响应时间过高"
          description: "P99 延迟 {{ $value | humanizeDuration }}，超过 2s 阈值"

      # QPS 突增（比过去 1 小时均值高 3 倍）
      - alert: TrafficSpike
        expr: |
          sum(rate(webook_http_requests_total[5m]))
          >
          3 * sum(rate(webook_http_requests_total[1h]))
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "请求量突增"
```

### Go 运行时告警

```yaml
groups:
  - name: webook-runtime
    rules:
      # goroutine 泄漏（超过 10000）
      - alert: GoroutineLeak
        expr: go_goroutines > 10000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "goroutine 数量异常"
          description: "当前 {{ $value }} 个 goroutine"

      # 内存持续增长
      - alert: MemoryGrowing
        expr: |
          predict_linear(go_memstats_alloc_bytes[1h], 3600) > 1e9
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "内存增长趋势异常"
          description: "预计 1 小时后堆内存达 {{ $value | humanize1024 }}B"

      # 进程重启
      - alert: ProcessRestarted
        expr: resets(process_cpu_seconds_total[5m]) > 0
        labels:
          severity: info
        annotations:
          summary: "webook 进程重启"
```

### MySQL 告警

```yaml
groups:
  - name: webook-mysql
    rules:
      # 连接数接近上限（> 80%）
      - alert: MysqlConnectionHigh
        expr: |
          mysql_global_status_threads_connected
          /
          mysql_global_variables_max_connections
          > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MySQL 连接数过高"

      # 慢查询突增
      - alert: MysqlSlowQueries
        expr: rate(mysql_global_status_slow_queries[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MySQL 慢查询增多"
```

### Redis 告警

```yaml
groups:
  - name: webook-redis
    rules:
      # 命中率下降（< 90%）
      - alert: RedisCacheHitLow
        expr: |
          rate(redis_keyspace_hits_total[5m])
          /
          (rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m]))
          < 0.9
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Redis 缓存命中率过低"

      # 内存使用过高
      - alert: RedisMemoryHigh
        expr: redis_memory_used_bytes > 100 * 1024 * 1024
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Redis 内存使用超过 100MB"
```

### Kafka 告警

```yaml
groups:
  - name: webook-kafka
    rules:
      # 消费 lag 持续增长
      - alert: KafkaConsumerLag
        expr: sum(kafka_consumergroup_lag) by (consumergroup, topic) > 1000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Kafka 消费 lag 过大"
          description: "{{ $labels.consumergroup }}/{{ $labels.topic }} lag={{ $value }}"

      # Broker 掉线
      - alert: KafkaBrokerDown
        expr: kafka_brokers < 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Kafka Broker 不可用"
```

### 通用目标存活告警

```yaml
groups:
  - name: targets
    rules:
      # 任何抓取目标挂了
      - alert: TargetDown
        expr: up == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.job }} 抓取失败"
          description: "实例 {{ $labels.instance }} 已 DOWN 超过 2 分钟"
```

## 告警级别约定

| 级别 | 含义 | 响应时间 |
|------|------|----------|
| `critical` | 服务不可用或数据丢失风险 | 立即 |
| `warning` | 性能下降或资源紧张 | 工作时间内处理 |
| `info` | 信息通知（重启、部署） | 知晓即可 |

## Alertmanager 集成（后续）

告警触发后需要 Alertmanager 做路由和通知：

```yaml
# docker-compose.yaml 新增
alertmanager:
  image: prom/alertmanager:v0.27.0
  ports: ["9093:9093"]
  volumes:
    - ./prometheus/alertmanager.yml:/etc/alertmanager/alertmanager.yml

# prometheus.yml 新增
alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

通知渠道：邮件、Slack、Webhook、企业微信等。
