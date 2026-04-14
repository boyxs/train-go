# Prometheus 监控文档

webook 项目 Prometheus 监控完整指南。

## 目录

| 文档 | 内容 |
|------|------|
| [01-architecture](01-architecture.md) | 架构总览、四种指标类型、webook 指标清单 |
| [02-deployment](02-deployment.md) | 部署启动、配置说明、验证排查 |
| [03-promql](03-promql.md) | PromQL 语法、函数详解、操作符 |
| [04-webook-queries](04-webook-queries.md) | webook 实战查询（HTTP/Go/MySQL/Redis/Kafka） |
| [05-alerting](05-alerting.md) | 告警规则模板（HTTP/运行时/MySQL/Redis/Kafka） |
| [06-best-practices](06-best-practices.md) | 命名规范、标签基数、PromQL 陷阱、排查清单、RED/USE 方法 |

## 快速开始

```bash
# 1. 启动监控栈
docker compose up -d prometheus grafana mysqld-exporter redis-exporter kafka-exporter

# 2. 确认 Targets 全部 UP
# 访问 http://虚拟机IP:9090/targets

# 3. 试几个查询
# 访问 http://虚拟机IP:9090 → 输入框写 PromQL
```

常用查询速查：

```promql
up                                    # 所有目标存活状态
rate(webook_http_requests_total[5m])  # QPS

histogram_quantile(0.99,              # P99 响应时间
  rate(webook_http_requests_duration_seconds_bucket[5m]))

go_goroutines                         # goroutine 数
go_memstats_alloc_bytes / 1024 / 1024 # 堆内存 MB
```
