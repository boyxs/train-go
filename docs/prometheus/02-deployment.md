# 部署与配置

## 一键启动

```bash
# 启动监控栈（Prometheus + Grafana + 3 个 Exporter）
docker compose up -d prometheus grafana mysqld-exporter redis-exporter kafka-exporter

# 查看状态
docker compose ps

# 查看日志
docker compose logs -f prometheus
```

## 访问地址

| 服务 | 地址 | 用途 |
|------|------|------|
| Prometheus UI | `http://虚拟机IP:9090` | PromQL 查询、Targets 状态 |
| Grafana | `http://虚拟机IP:3001` | 可视化面板（admin/admin） |
| webook /metrics | `http://localhost:8010/metrics` | 原始指标（给 Prometheus 抓的） |

## 配置文件说明

### prometheus.yml（Docker 版）

```yaml
# deploy/prometheus/prometheus.yml（项目路径 work/deploy/prometheus/）
global:
  scrape_interval: 15s      # 默认抓取间隔
  evaluation_interval: 15s   # 规则评估间隔

scrape_configs:
  - job_name: 'prometheus'           # Prometheus 自身
    static_configs:
      - targets: ['localhost:9090']

  - job_name: 'webook-app'           # webook 应用
    metrics_path: '/metrics'
    static_configs:
      - targets: ['host.docker.internal:8010']  # Docker 容器访问宿主机

  - job_name: 'mysql'
    static_configs:
      - targets: ['mysqld-exporter:9104']

  - job_name: 'redis'
    static_configs:
      - targets: ['redis-exporter:9121']

  - job_name: 'kafka'
    static_configs:
      - targets: ['kafka-exporter:9308']
```

### prometheus.local.yml（本地版）

Prometheus 二进制跑在宿主机，所有目标走 localhost：

```bash
prometheus.exe --config.file=prometheus/prometheus.local.yml
```

区别：targets 全部是 `localhost:端口`，需要 exporter 的端口映射到宿主机。

### 关键配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `scrape_interval` | 15s | 抓取间隔，生产建议 15s~30s |
| `scrape_timeout` | 10s | 单次抓取超时 |
| `evaluation_interval` | 15s | 告警规则评估间隔 |
| `metrics_path` | /metrics | 抓取路径 |

## Docker Compose 服务说明

### Prometheus

```yaml
prometheus:
  image: prom/prometheus:v2.51.0
  ports: ["9090:9090"]
  volumes:
    - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml  # 配置
    - prometheus-data:/prometheus                                 # 数据持久化
  extra_hosts:
    - "host.docker.internal:host-gateway"  # Linux 下容器访问宿主机
```

### Exporter 说明

| Exporter | 作用 | 原理 |
|----------|------|------|
| mysqld-exporter | 采集 MySQL 指标 | 连接 MySQL 执行 `SHOW GLOBAL STATUS` 等 |
| redis-exporter | 采集 Redis 指标 | 连接 Redis 执行 `INFO` 命令 |
| kafka-exporter | 采集 Kafka 指标 | 连接 Kafka 读取集群元数据和消费组信息 |

Exporter 不对外暴露端口，只在 Docker 网络内供 Prometheus 抓取。

## 验证部署

### 1. 检查 Targets

访问 `http://虚拟机IP:9090/targets`，确认所有 job 状态为 **UP**：

| Job | 状态 | 排查 |
|-----|------|------|
| prometheus | UP | Prometheus 自身，不会失败 |
| webook-app | UP | 确认 webook 在跑，/metrics 可访问 |
| mysql | UP | 确认 mysqld-exporter 容器运行中 |
| redis | UP | 确认 redis-exporter 容器运行中 |
| kafka | UP | 确认 kafka-exporter 容器运行中 |

### 2. 测试查询

在 Prometheus UI 的查询框输入 `up`，应看到所有 job 的值为 1。

### 3. 常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| webook-app 显示 DOWN | webook 没启动，或 /metrics 被 JWT 拦截 | 确认 webook 在跑；/metrics 路由在中间件之前注册 |
| 401 Unauthorized | /metrics 被认证中间件拦截 | 确认 `server.GET("/metrics", ...)` 在 `server.Use(middlewares...)` 之前 |
| Connection refused | 目标端口没监听 | 检查服务是否启动、端口映射是否正确 |
| Context deadline exceeded | 抓取超时 | 检查网络连通性，调大 `scrape_timeout` |

## Grafana 配置

### 数据源（已自动配置）

`deploy/grafana/provisioning/datasources/prometheus.yml` 自动将 Prometheus 配为默认数据源。

### 导入面板

Dashboards → New → Import → 填 ID → Load → 选 Prometheus 数据源：

| ID | 名称 | 看什么 |
|----|------|--------|
| 6671 | Go Processes | goroutine、GC、内存 |
| 14031 | Gin Prometheus | HTTP QPS、延迟、状态码 |
| 14057 | MySQL Overview | 连接数、QPS、慢查询 |
| 11835 | Redis Dashboard | 命中率、内存、连接 |
| 7589 | Kafka Exporter | 消费 lag、分区偏移 |

## 数据保留

Prometheus 默认保留 15 天数据。修改：

```yaml
prometheus:
  command:
    - '--config.file=/etc/prometheus/prometheus.yml'
    - '--storage.tsdb.retention.time=30d'   # 保留 30 天
    - '--storage.tsdb.retention.size=5GB'   # 或限制磁盘占用
```
