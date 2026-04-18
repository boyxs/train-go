# Exporter 选型

Exporter 决定 span **发往哪里、用什么协议**。换 exporter 业务代码零改动，是 OTel 最大的价值之一。

## 一、四种主流 Exporter

| Exporter | 协议 | 包路径 | 用途 |
|----------|------|--------|------|
| **stdout** | 打印到 stdout（JSON） | `exporters/stdout/stdouttrace` | 学习 / 单元测试 / 调试 |
| **Zipkin** | HTTP POST + Zipkin v2 JSON | `exporters/zipkin` | 直连 Zipkin |
| **OTLP/HTTP** | HTTP + Protobuf | `exporters/otlp/otlptrace/otlptracehttp` | 接 Collector / Tempo / 商业 APM |
| **OTLP/gRPC** | gRPC + Protobuf | `exporters/otlp/otlptrace/otlptracegrpc` | 同上，性能更好 |

> **Jaeger exporter 已废弃**：`exporters/jaeger`（Thrift 协议）在 OTel v1.17+ 标记 Deprecated。新代码统一用 OTLP，Jaeger v1.35+ 原生支持 OTLP 接收。

## 二、对比表

| 维度 | stdout | Zipkin | OTLP/HTTP | OTLP/gRPC |
|------|--------|--------|-----------|-----------|
| 后端要求 | 无 | Zipkin / 兼容服务 | Collector / OTLP 后端 | Collector / OTLP 后端 |
| 协议 | 无 | HTTP + JSON | HTTP/1.1 + Protobuf | HTTP/2 + Protobuf |
| 性能 | 低（IO 瓶颈） | 中 | 中高 | **最高** |
| 防火墙友好 | 不需要 | ✅ 80/443 可走 | ✅ 80/443 可走 | ❌ 需要 H2 |
| 跨语言互通 | / | 仅 Zipkin 生态 | ✅ OTel 标准 | ✅ OTel 标准 |
| 调试友好度 | ✅ 直接看 | ✅ wire 抓包能看 | ❌ 二进制 | ❌ 二进制 |
| 推荐场景 | 本地开发 | 简单部署、轻量 | 生产（穿越 LB/Ingress） | 生产（同 VPC，性能优先）|

## 三、stdout exporter

### 用途
- 测试用例验证 span 生成
- 本地 debug 看 span 长什么样
- 教学

### 用法

```go
import "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"

exporter, _ := stdouttrace.New(
    stdouttrace.WithPrettyPrint(),                // JSON 缩进
    // stdouttrace.WithWriter(file),              // 写到文件而非 stdout
    // stdouttrace.WithoutTimestamps(),           // 不输出时间戳（diff 友好）
)
```

### 利弊
- **优点**：零依赖、零网络、所见即所得
- **缺点**：高 QPS 时 stdout 写入会成为瓶颈；不要用于生产

## 四、Zipkin exporter

### 用途
- 已有 Zipkin 集群（小公司/老项目常见）
- 想要轻量后端（Zipkin 单 jar 包能跑，对资源要求低）

### 用法

```go
import "go.opentelemetry.io/otel/exporters/zipkin"

exporter, _ := zipkin.New(
    "http://zipkin:9411/api/v2/spans",
    zipkin.WithClient(&http.Client{Timeout: 5 * time.Second}),
)

tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),               // 必须用 Batch
)
```

### 利弊
- **优点**：Zipkin 后端轻量（200MB 内存能跑）；HTTP+JSON 易调试
- **缺点**：UI 简陋（对比 Jaeger）；生态主要在 Java 圈；非 OTLP 标准协议

## 五、OTLP exporter（推荐生产用）

### OTLP/HTTP

```go
import (
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
)

exporter, _ := otlptracehttp.New(ctx,
    otlptracehttp.WithEndpoint("collector:4318"),
    otlptracehttp.WithInsecure(),                 // 禁 TLS（内网）
    // otlptracehttp.WithHeaders(map[string]string{
    //     "Authorization": "Bearer " + token,    // 商业 APM 鉴权
    // }),
)
```

### OTLP/gRPC

```go
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

exporter, _ := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("collector:4317"),
    otlptracegrpc.WithInsecure(),
)
```

### 端口约定
- OTLP/gRPC：**4317**
- OTLP/HTTP：**4318**

### 利弊
- **优点**：CNCF 标准协议；所有商业 APM（Datadog / NewRelic / Honeycomb / 阿里 ARMS）都支持；Collector 中转生态成熟
- **缺点**：必须有 Collector 或兼容后端；二进制协议调试不直观

## 六、是否引入 Collector

```
方案 A：应用 → 后端          方案 B：应用 → Collector → 后端
直连，简单                   多一跳，但解耦
```

| 维度 | 直连 | Collector |
|------|------|-----------|
| 部署复杂度 | 简单 | 多一个进程 |
| 应用配置 | 改代码（exporter 地址） | 改 Collector 配置 |
| 多后端 fan-out | 不支持（要写多个 exporter） | 支持（一份数据发多家）|
| 尾部采样 | 不支持 | 支持 |
| 协议转换 | 不支持 | 支持（OTLP 进，Zipkin 出） |
| 重试 / 背压 | SDK 内 | Collector 顶住 |
| **适合** | 学习、单服务 demo、Zipkin 直连 | 生产、多服务、要换后端时 |

webook 当前阶段：**直连 Zipkin/Jaeger 即可**，等服务扩到 5 个以上、或要接商业 APM，再加 Collector。

## 七、推荐组合

| 场景 | 选型 |
|------|------|
| 本地开发 / 单元测试 | stdout |
| 单机 demo / 学习 | Zipkin（docker run 一行起） |
| 生产（自建） | Jaeger v2 + OTLP/gRPC（中等规模）/ Tempo + Grafana（大规模） |
| 生产（商业） | OTLP/HTTP → Datadog / NewRelic / Honeycomb |
| 多后端共存 | OTLP/gRPC → Collector → 多 exporter |

## 八、Exporter 失败时怎么办

所有 exporter 都可能因为网络/后端故障失败，OTel SDK 的处理：

1. **BatchSpanProcessor**：内部有重试（默认 5 次，指数退避）
2. 重试失败 → span 丢弃，错误打到 OTel 的 ErrorHandler（默认打 stderr）
3. 队列满（默认 2048）→ 后续 span 直接丢

**配置自定义错误处理**：

```go
otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
    log.Printf("OTel error: %v", err)             // 接到自家日志系统
}))
```

**生产监控**：观测 Collector 的 `otelcol_exporter_send_failed_spans` 指标，设告警。
