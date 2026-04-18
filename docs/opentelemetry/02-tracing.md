# Tracing 核心模型详解

理解这一章后，看任何 OTel 代码都不再蒙圈。所有概念围绕**一个 Span 是怎么从代码里产生、最后落到 Jaeger UI 的**这条路径展开。

## 一、整体数据流

```
业务代码                SDK 内部                          网络
────────                ────────                          ────
otel.Tracer("foo")  ─►  TracerProvider.Tracer("foo")
                           │
tracer.Start(ctx, "op") ─► 创建 Span（分配 SpanID）
                           │  ParentSpan ← 从 ctx 取
                           │  采样决策 ← Sampler
                           │  附加 Resource（service.name）
                           │
span.SetAttributes(...) ─► 写入 Span 内存对象
span.AddEvent(...)
span.End()              ─► 交给 SpanProcessor
                              │
                              ├── SimpleSpanProcessor:  立即调 Exporter（同步）
                              └── BatchSpanProcessor:   入队 → 攒批 → 后台 goroutine 调 Exporter
                                       │
                                       ▼
                                   Exporter.ExportSpans([]Span)
                                       │
                                       ▼
                              [stdout / Zipkin HTTP / OTLP gRPC / ...]
```

记住这条主线，下面每个概念都对号入座。

## 二、TracerProvider：根工厂

整个进程一般只有一个 `TracerProvider`，是所有 Tracer 的工厂，也是采样、处理、导出策略的承载者。

```go
tp := sdktrace.NewTracerProvider(
    sdktrace.WithResource(res),                  // 服务标识
    sdktrace.WithSampler(sdktrace.AlwaysSample()), // 采样策略
    sdktrace.WithBatcher(exporter),              // SpanProcessor + Exporter
)
otel.SetTracerProvider(tp)                       // 注册为全局
defer tp.Shutdown(ctx)                           // 关闭：flush + 释放
```

**关键点**：
- 必须 `Shutdown`，否则 BatchSpanProcessor 队列里的 span 会丢
- 可以不 `SetTracerProvider`，但全局注册后业务代码 `otel.Tracer("xxx")` 才能拿到
- 同一进程多个 Provider 也允许（少见，比如多租户隔离），通过 `tp.Tracer()` 显式拿

## 三、Resource：服务身份证

Resource 是**附加到所有 Span 上的固定标签**，描述"这条 span 来自谁"。

```go
res, _ := resource.Merge(
    resource.Default(),
    resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceName("webook-api"),       // 必填，UI 用它分组
        semconv.ServiceVersion("v1.2.3"),
        semconv.DeploymentEnvironment("prod"),
        attribute.String("k8s.pod.name", "webook-api-7df9b-abcde"),
    ),
)
```

**为什么要用 `semconv` 而不是手写字符串**：
- `semconv` 是 OTel **语义约定（Semantic Conventions）** 的 Go 绑定，键名标准化
- 用了 `semconv.ServiceName(...)` 后，所有后端（Jaeger/Zipkin/Datadog）都能正确识别为"服务名"
- 自己写 `attribute.String("service", "...")` 后端不认，UI 显示就不对

**包路径形如** `go.opentelemetry.io/otel/semconv/v1.26.0`，版本号代表遵循的 OTel 语义约定版本。新版本只是字段更全，按需升级即可。

## 四、Tracer：业务模块的入口

```go
tracer := otel.Tracer("webook/internal/service/article")
```

`Tracer` 是创建 Span 的入口，**字符串参数是 instrumentation scope（仪表化作用域）名**——一般用包路径或库名。后端 UI 用它做分组过滤（"看看所有 article 模块的 span"）。

实践：每个包用一个 Tracer 即可，不要为每个函数 new 一个。

## 五、Span：一次操作的记录

### 5.1 创建与结束

```go
ctx, span := tracer.Start(ctx, "ArticleService.Publish",
    trace.WithSpanKind(trace.SpanKindServer),     // span 类型
    trace.WithAttributes(
        attribute.Int64("article.id", id),
    ),
)
defer span.End()                                   // 必须 End，否则不会上报
```

**`Start` 返回新 ctx**：把新 span 写进了 ctx，后续从这个 ctx start 的子 span 会自动挂到当前 span 下。**所以传 ctx 必须传新的**，不能传老的。

### 5.2 SpanKind（Span 类型）

| Kind | 含义 | 举例 |
|------|------|------|
| `Internal` | 默认值，进程内调用 | service 调 repo |
| `Server` | 接收远程请求 | HTTP handler、gRPC server method |
| `Client` | 发起远程请求 | http.Get、gRPC client call |
| `Producer` | 发消息（异步） | Kafka producer.Send |
| `Consumer` | 收消息（异步） | Kafka consumer.Poll |

后端 UI 会按 Kind 画不同图标，跨服务 trace 拼接也依赖它。

### 5.3 Attributes（属性）

Span 上的键值对，用于过滤、聚合、展示。

```go
span.SetAttributes(
    attribute.String("http.method", "POST"),
    attribute.String("http.route", "/articles/:id"),
    attribute.Int("http.status_code", 200),
    attribute.Int64("user.id", 1024),
)
```

**约定**：使用 semconv 包里的标准键名，自定义键加业务前缀（`webook.article.id`）。

### 5.4 Events（事件）

Span 上的时间点标记，类似日志，但挂在 span 上。

```go
span.AddEvent("cache.miss", trace.WithAttributes(
    attribute.String("key", "user:1024"),
))
```

**典型用途**：
- 异常分支记录（`AddEvent("fallback to backup")`）
- 重试节点（`AddEvent("retry", attribute.Int("attempt", 2))`）
- 缓存命中/未命中标记

### 5.5 Status（状态）

```go
span.SetStatus(codes.Ok, "")          // 默认 Unset，显式标 Ok
span.SetStatus(codes.Error, "DB 超时") // 错误，UI 标红
span.RecordError(err)                  // 等价于 AddEvent("exception") + 把 err 信息写进事件属性
```

**RecordError 不会自动 SetStatus(Error)**。完整的错误处理：

```go
if err != nil {
    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
    return err
}
```

## 六、Context 传播：Span 怎么串起来

### 6.1 进程内：用 `context.Context`

OTel 把当前 active span 塞进 `context.Context`。只要把 ctx 一路透传，子 span 自动挂到父 span 下。

```go
func A(ctx context.Context) {
    ctx, span := tracer.Start(ctx, "A")
    defer span.End()
    B(ctx)                       // ✅ 透传 ctx，B 的 span 是 A 的子
    // C(context.Background())   // ❌ 断链，C 会变成新的 root span
}
```

**断链是新手最常见的 bug**。任何"重新拿一个 ctx"的写法都会让 trace 断成两截。

### 6.2 跨进程：用 Propagator 写 HTTP/gRPC header

进程 A 的 span 要让进程 B 知道，必须在网络请求里捎带 SpanContext。OTel 通过 `propagation.TextMapPropagator` 做注入和提取。

```go
// 全局设一次（推荐 W3C TraceContext 标准）
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
    propagation.TraceContext{},   // traceparent / tracestate header
    propagation.Baggage{},        // baggage header
))

// Client 端：把 ctx 里的 SpanContext 写进 HTTP header
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

// Server 端：从 header 提取 SpanContext，放进 ctx
ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
ctx, span := tracer.Start(ctx, "handler")
```

**W3C TraceContext** header 长这样：

```
traceparent: 00-7c2e4dd1f8a04c8fae8b5e7b7c0b6b3a-01a2b3c4d5e6f708-01
             │  │                                │                │
             │  │                                │                └─ flags（采样位）
             │  │                                └─ ParentSpanID
             │  └─ TraceID
             └─ version
```

实际开发不用手写，`otelhttp` / `otelgin` / `otelgrpc` 中间件自动做 Inject/Extract。

## 七、Sampler：采样策略

全采样在大流量下不可承受（trace 量爆炸 + 网络/存储成本）。Sampler 决定**这条 trace 要不要留**。

| Sampler | 行为 | 适用 |
|---------|------|------|
| `AlwaysSample()` | 全留 | 开发/测试 |
| `NeverSample()` | 全丢 | 临时关闭 |
| `TraceIDRatioBased(0.1)` | 按 TraceID 哈希采 10% | 生产基线 |
| `ParentBased(...)` | 跟随父 span 决策（父采则采） | 生产**必选包装** |

**生产推荐组合**：

```go
sdktrace.WithSampler(
    sdktrace.ParentBased(
        sdktrace.TraceIDRatioBased(0.1),       // 没父 span 时按 10% 采
    ),
)
```

为什么必须 `ParentBased`：跨服务时如果上游已经决定采样，下游必须跟随，否则同一条 trace 一半在一半不在，毫无意义。

### 头部采样 vs 尾部采样

- **头部采样（Head Sampling）**：在 root span 创建时立刻决定。SDK 自带的都是这种。优点：开销小；缺点：决定时不知道这条 trace 后面有没有错。
- **尾部采样（Tail Sampling）**：等整条 trace 完成再决定（"凡是有错的都留"）。需要在 OTel Collector 配置 `tail_sampling` processor 实现。

## 八、SpanProcessor：批 vs 同步

| Processor | 行为 | 适用 |
|-----------|------|------|
| `SimpleSpanProcessor` | span End → 立刻同步调 Exporter | 测试 / 单元测试 |
| `BatchSpanProcessor` | span End → 入队 → 后台 goroutine 攒批发送 | **生产唯一选项** |

```go
// 测试
sdktrace.WithSyncer(exporter)             // 内部包成 SimpleSpanProcessor

// 生产
sdktrace.WithBatcher(exporter,            // 内部包成 BatchSpanProcessor
    sdktrace.WithMaxQueueSize(2048),
    sdktrace.WithBatchTimeout(5*time.Second),
    sdktrace.WithMaxExportBatchSize(512),
)
```

**注意**：BatchSpanProcessor 队列满会丢 span（默认 2048）。日志里出现 `OTel ERROR: max queue size reached` 就要调大队列或检查后端为什么慢。

## 九、Exporter：往哪发

四种主流，详见 `04-exporters.md`。

| Exporter | 协议 | 用途 |
|----------|------|------|
| `stdouttrace` | 打印到控制台 | 学习 / 调试 |
| `zipkin` | HTTP + Zipkin JSON | 接 Zipkin |
| `otlptracehttp` / `otlptracegrpc` | OTLP（HTTP/gRPC + Protobuf） | 接 Collector / Tempo / Jaeger v2 / 商业 APM |
| `jaeger`（已废弃） | Thrift | 旧 Jaeger，**新代码用 OTLP 走 Collector** |

## 十、一图回顾

```
进程启动:
  Resource ──┐
  Sampler ──┤
  Exporter ─┴─► TracerProvider ──全局注册──► otel.SetTracerProvider(tp)

请求处理（HTTP server）:
  Request ──header──► Propagator.Extract ──► ctx (带 parent SpanContext)
        ──► tracer.Start(ctx, "handler") ──► span (Kind=Server)
            ├─ service 层: tracer.Start(ctx, "Service.X") ──► child span
            ├─ DAO 层: tracer.Start(ctx, "MySQL SELECT") ──► child span (Kind=Client)
            └─ HTTP 调外部: Propagator.Inject(ctx, headers) ──► 跨进程传播
        ──► span.End() ──► BatchSpanProcessor ──► Exporter ──► 后端

进程退出:
  tp.Shutdown(ctx) ──► flush 队列里所有 span
```

下一篇 `03-quickstart.md` 用 `sandbox/opentelemetry/` 测试代码逐行讲解。
