# OpenTelemetry 概念与架构

## 一、OpenTelemetry 是什么

**OpenTelemetry（简称 OTel）** 是 CNCF 托管的一套**可观测性数据采集标准 + SDK + 协议**，由 OpenTracing 和 OpenCensus 合并而来，2021 年成为 CNCF 孵化项目，目前是 CNCF 仅次于 Kubernetes 的第二活跃项目。

它解决的问题：**统一可观测性数据的产生、传输、协议**，让"采集"和"后端存储"解耦。

```
旧时代（厂商绑定）：
  应用 ──Jaeger 客户端──► Jaeger
  应用 ──Zipkin 客户端──► Zipkin
  应用 ──SkyWalking SDK──► SkyWalking
  → 换后端要改全部代码

OTel 时代（统一抽象）：
  应用 ──OTel SDK──► OTLP 协议 ──► [Jaeger / Zipkin / Tempo / Datadog / ...]
  → 换后端只换 exporter，业务代码不动
```

## 二、三大信号（Three Pillars / Signals）

OTel 把可观测性数据分成三类信号：

| 信号 | 英文 | 回答的问题 | 数据形态 | 典型后端 |
|------|------|-----------|---------|---------|
| **追踪** | Traces | 一次请求经过哪些服务、各花了多久 | 树状 Span | Jaeger / Zipkin / Tempo |
| **指标** | Metrics | 系统整体的 QPS / 延迟 / 错误率 | 时间序列（数值） | Prometheus / VictoriaMetrics |
| **日志** | Logs | 具体某一时刻发生了什么（详细文本） | 结构化文本 | Loki / ES / ClickHouse |

**信号关联**：三者通过 `trace_id`、`span_id` 串起来。日志里带上 trace_id，从 Grafana 指标 → Jaeger trace → Loki 日志一气呵成。

> **本文档主要讲 Traces**。Metrics 我们已经用 Prometheus 实现了（见 `docs/prometheus/`），OTel 的 Metric SDK 后续如果需要再补。Logs 一般继续用 zap/logrus，挂上 trace_id 即可。

## 三、整体架构

```
┌──────────────────────┐
│      Application     │
│  ┌────────────────┐  │
│  │  OTel API      │  │  ← 业务代码只依赖 API，不依赖 SDK
│  │  (otel.Tracer) │  │
│  └────────┬───────┘  │
│           │           │
│  ┌────────▼───────┐  │
│  │  OTel SDK      │  │  ← 实现采样、批处理、资源标注
│  │  (TracerProvider)│ │
│  └────────┬───────┘  │
│           │           │
│  ┌────────▼───────┐  │
│  │  Exporter      │  │  ← stdout / OTLP / Zipkin / Jaeger
│  └────────┬───────┘  │
└───────────┼──────────┘
            │ OTLP (gRPC/HTTP)
            ▼
   ┌──────────────────┐
   │  OTel Collector  │  ← 可选中转层：批处理、过滤、路由、协议转换
   │  (receiver       │
   │   → processor    │
   │   → exporter)    │
   └────────┬─────────┘
            │
   ┌────────┴─────────┬─────────────┐
   ▼                  ▼             ▼
┌────────┐       ┌────────┐    ┌──────────┐
│ Jaeger │       │ Zipkin │    │ Datadog  │
└────────┘       └────────┘    └──────────┘
```

### 分层职责

| 层 | 角色 | webook 现阶段选型 |
|----|------|-------------------|
| **API** | 业务代码调用的接口（`otel.Tracer().Start()`），稳定不变 | `go.opentelemetry.io/otel` |
| **SDK** | 实现层：采样、批处理、资源标识 | `go.opentelemetry.io/otel/sdk` |
| **Exporter** | 把 span 序列化发送出去 | `stdouttrace`（学习）/ `zipkin`（演示）|
| **Collector** | 独立进程，接收→处理→转发，应用与后端解耦 | 暂不引入，直接 SDK → Zipkin |
| **Backend** | 存储 + UI | Zipkin（轻量）/ Jaeger（功能全） |

### 为什么要 Collector？

应用直连后端是最简方案，但生产环境通常加一层 Collector：

- **解耦**：换后端不用改应用配置；多后端 fan-out（同一份数据同时发 Jaeger 和 Datadog）
- **批处理 / 重试**：Collector 顶住后端抖动，应用 SDK 只负责快速吐出
- **过滤 / 采样**：在 Collector 层做尾部采样（看到完整 trace 再决定要不要留）
- **协议转换**：应用发 OTLP，Collector 转 Zipkin/Jaeger 格式

## 四、Trace 模型核心术语

```
TraceID = 7c2e... (整条调用链唯一)
│
├─ Span A "POST /order"  (root span，服务 web)
│  ├─ SpanID = 01..., ParentSpanID = (空)
│  ├─ Span B "OrderService.Create"  (服务 web)
│  │  ├─ SpanID = 02..., ParentSpanID = 01...
│  │  ├─ Span C "MySQL INSERT order"  (服务 web → MySQL)
│  │  │  └─ SpanID = 03..., ParentSpanID = 02...
│  │  └─ Span D "Kafka publish order.created"
│  │     └─ SpanID = 04..., ParentSpanID = 02...
│  └─ Span E "send response"
│     └─ SpanID = 05..., ParentSpanID = 01...
```

| 术语 | 含义 |
|------|------|
| **Trace** | 一次完整请求的所有 Span 集合，由 TraceID 标识 |
| **Span** | 一次操作的记录：操作名 + 起止时间 + 属性 + 事件 + 状态 |
| **TraceID / SpanID** | 16 字节 / 8 字节随机数，全局唯一 |
| **ParentSpanID** | 父 Span ID，构成树状关系（root span 的 ParentSpanID 为空）|
| **SpanContext** | TraceID + SpanID + TraceFlags + TraceState，跨进程透传的最小单元 |
| **Baggage** | 跟随 SpanContext 透传的业务键值对（如 `user_id`），用于跨服务传递业务信息 |

## 五、与 Prometheus 的关系

**不是替代关系，是互补关系。**

| 场景 | 用 Prometheus | 用 OTel Tracing |
|------|---------------|-----------------|
| 大盘监控（QPS / 错误率 / P99） | ✅ | ❌（trace 不适合做聚合） |
| 告警（错误率涨了发钉钉） | ✅ | ❌ |
| 单笔请求慢，定位到哪一步 | ❌（只有聚合数据） | ✅ |
| 跨服务调用链可视化 | ❌ | ✅ |
| 分析"为什么这个用户的请求失败了" | ❌ | ✅ |

**典型协作流程**：
1. Grafana 看到 P99 飙升 → Prometheus 告警
2. 在 Grafana 高延迟点点击 exemplar → 跳到 Jaeger 对应 trace
3. Jaeger 看 span 火焰图 → 发现 MySQL 那段 800ms
4. 拿 trace_id 去 Loki 查日志 → 看到 SQL 内容、参数
5. 定位问题：缺索引

OTel 的目标是让这个流程**协议层统一**，而不是让一个工具干所有事。

## 六、OTel 的利与弊

### 优点

- **厂商中立**：换后端只换 exporter，业务代码零改动
- **生态广**：主流框架都有 instrumentation（gin/gorm/grpc/redis/kafka 等开箱即用）
- **多语言一致**：Go / Java / Python / Node 概念和 API 风格统一
- **协议标准（OTLP）**：基于 gRPC / HTTP + Protobuf，性能好，跨语言互通

### 缺点 / 代价

- **学习曲线**：概念多（Provider / Processor / Exporter / Sampler / Resource ...），上手门槛比直接用 Zipkin SDK 高
- **依赖体积**：sdk + exporter 加起来几 MB，二进制变大
- **运行时开销**：每个 span 创建/结束都有成本（约几 μs），全采样高 QPS 服务要算账
- **版本演进快**：v1.x 之前 API 频繁变动，需要锁定版本（webook 锁 v1.32.0）
- **Logs 信号还在 Stable 边缘**：Trace/Metric 已 GA，Log SDK 部分语言仍在 Beta

### 何时**不需要** OTel

- 单体应用、没有跨进程调用 → 一个 zap 日志够用
- QPS 极高且服务简单 → 引入开销不划算，用 Prometheus + 关键路径打日志
- 团队没有人维护 trace 后端 → 没人看的 trace 等于没有

webook 是多服务架构（web → service → repo → MySQL/Redis/Kafka/ES），引入 OTel 收益明显。
