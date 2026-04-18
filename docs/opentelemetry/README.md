# OpenTelemetry 学习与实战文档

webook 项目 OpenTelemetry（OTel）完整学习指南。配套示例代码：`opentelemetry/`（独立 Go 模块 `otel-demo`）。

## 目录

| 文档 | 内容 |
|------|------|
| [01-concepts](01-concepts.md) | OTel 是什么、三大信号（Trace/Metric/Log）、整体架构、与 Prometheus 的关系 |
| [02-tracing](02-tracing.md) | Tracing 核心模型：TracerProvider / Tracer / Span / Context / Sampler / Processor / Exporter |
| [03-quickstart](03-quickstart.md) | 基于 `opentelemetry/` 测试代码的上手教程（stdout 版） |
| [04-exporters](04-exporters.md) | stdout / Zipkin / Jaeger / OTLP 四种 exporter 对比与选型 |
| [05-integration](05-integration.md) | 接入 webook：otelgin / otelgorm / otelredis / 跨服务传播 |
| [06-best-practices](06-best-practices.md) | Span 命名、属性、采样策略、利弊、踩坑清单 |

## 一句话理解

> **Prometheus 回答"系统现在什么样"（指标聚合），OpenTelemetry 回答"这一次请求经历了什么"（调用链追踪）。两者互补，不替代。**

## 快速开始

```bash
# 1. 跑示例测试（stdout，无外部依赖）
cd C:/Go/work/opentelemetry
go test -v -run TestTracer

# 2. 起本地 Zipkin，看 UI
docker run -d -p 9411:9411 openzipkin/zipkin
go test -v -run TestZipkin
# 浏览器访问 http://localhost:9411
```

## 核心概念速查

```
TracerProvider  ──创建──►  Tracer  ──创建──►  Span
     │                                          │
     ├── Resource（服务标识：service.name 等）   ├── Attributes（键值对）
     ├── Sampler（采样策略）                    ├── Events（时间点事件）
     ├── SpanProcessor（批/同步处理）           ├── Status（Ok / Error）
     └── Exporter（stdout/Zipkin/OTLP/...）     └── SpanContext（TraceID / SpanID）
```

## 与 Prometheus 监控栈的关系

| 维度 | Prometheus（已落地） | OpenTelemetry（本次扩展） |
|------|---------------------|---------------------------|
| 关心什么 | 系统整体状态：QPS / 错误率 / 延迟分位数 | 单次请求路径：A → B → C 各花了多久 |
| 数据形态 | 时间序列（数值） | 调用链（树状 Span） |
| 模型 | Pull（Prometheus 拉 `/metrics`） | Push（应用推到 Collector / 后端） |
| 后端 | Prometheus + Grafana | Jaeger / Zipkin / Tempo / 商业 APM |
| 适合排查 | "整体慢了" / "错误率涨了" | "这一笔订单为什么慢" / "调用链哪一环出错" |

两者通过 **`exemplar`** 机制可以打通：Prometheus Histogram 桶里挂上 TraceID，从 Grafana 高延迟点直接跳到对应 trace。
