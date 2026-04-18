# Grafana 学习与生产实战文档

webook 项目 Grafana 完整指南。当前部署：`webook-grafana`（v11.6.0），端口 `3001`，配置 `grafana/provisioning/`（顶层）。

## 目录

| 文档 | 内容 |
|------|------|
| [01-concepts](01-concepts.md) | Grafana 是什么、能干什么、与 Prometheus/Loki/Tempo 关系、利弊 |
| [02-deployment](02-deployment.md) | 部署、Provisioning、关键配置项 |
| [03-datasources](03-datasources.md) | 数据源管理（Prometheus / Loki / Tempo / MySQL） |
| [04-dashboards](04-dashboards.md) | 面板设计、变量、模板、JSON model |
| [05-alerting](05-alerting.md) | Grafana Alerting（vs Prometheus Alertmanager） |
| [06-production-workflow](06-production-workflow.md) | **生产级流程**：Dashboard as Code / Git / CI / 权限 / 备份 / 升级 |
| [07-best-practices](07-best-practices.md) | 性能、命名、协作、踩坑清单 |

## 一句话理解

> **Grafana 是数据可视化与告警的统一前端**：底层数据存在 Prometheus / Loki / Tempo / MySQL 等，Grafana 负责"画图、看图、告警、协作"。

## 快速开始

```bash
# 1. 启动（webook 项目已配好）
cd C:/Go/work/webook
docker compose up -d grafana prometheus

# 2. 访问 http://虚拟机IP:3001（默认 admin/admin）

# 3. 数据源已通过 provisioning 自动加载（Prometheus）
# 4. 推荐 dashboard 模板：见 grafana/provisioning/dashboards/README.md
```

## 文档优先级阅读路径

| 角色 | 推荐顺序 |
|------|---------|
| 第一次接触 | README → 01 → 02 → 04 |
| 要做生产部署 | 02 → 03 → 06 → 07 |
| 要写告警 | 05 → 07 |
| 要把 dashboard 纳管 Git | 06 |

## 核心思想

```
数据源（DS）──查询──► Grafana ──渲染──► Dashboard / Panel
                       │
                       ├── Variable（动态过滤：instance/env/...）
                       ├── Alert Rule（基于查询结果告警）
                       ├── Annotation（事件标注：发布、故障）
                       └── Provisioning（配置即代码）
```

记住一条：**Grafana 不存数据，只查数据**。所有问题先问"这个图背后的数据源是谁、查询是什么"。
