# 上手教程（基于示例代码）

配套示例：`C:/Go/work/sandbox/opentelemetry/`，独立 Go 模块 `otel-demo`。本章逐段拆解 `tracer_test.go`，跑通后你就有了完整的 Tracing 心智模型。

## 一、模块结构

```
opentelemetry/
├── go.mod              # 独立模块，依赖 otel sdk v1.32.0
├── tracer_test.go      # stdout exporter 示例（无外部依赖）
└── zipkin_test.go      # Zipkin exporter 示例（需起 Zipkin）
```

## 二、依赖说明

```go
require (
    github.com/stretchr/testify v1.11.1
    go.opentelemetry.io/otel v1.32.0                                   // API
    go.opentelemetry.io/otel/sdk v1.32.0                               // SDK 实现
    go.opentelemetry.io/otel/trace v1.32.0                             // Trace API 类型
    go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.32.0      // stdout exporter
    go.opentelemetry.io/otel/exporters/zipkin v1.32.0                  // zipkin exporter
)
```

**版本锁定**：OTel 各包必须使用相同 minor 版本，混用会报 schema 不兼容。升级时统一升。

## 三、stdout 版逐行拆解

### 3.1 初始化 TracerProvider

```go
func initTracer(t *testing.T) (trace.Tracer, func()) {
    // ① 创建 exporter
    exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
    require.NoError(t, err)

    // ② 创建 Resource（服务身份）
    res, err := resource.Merge(
        resource.Default(),                            // 自带 telemetry.sdk.* 等
        resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName("otel-demo"),
            semconv.ServiceVersion("v0.0.1"),
        ),
    )
    require.NoError(t, err)

    // ③ 装配 TracerProvider
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(exporter),                 // 测试用同步，立刻看到输出
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
    )

    // ④ 全局注册
    otel.SetTracerProvider(tp)

    // ⑤ 返回 tracer 和清理函数
    tracer := tp.Tracer("otel-demo/tracer_test")
    return tracer, func() {
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
        defer cancel()
        _ = tp.Shutdown(ctx)
    }
}
```

**对应 02 章的概念**：

| 代码 | 概念 |
|------|------|
| `stdouttrace.New(...)` | Exporter |
| `resource.Merge(...)` | Resource（服务身份证）|
| `WithSyncer` | SimpleSpanProcessor（测试同步导出） |
| `AlwaysSample()` | Sampler（学习场景全采样）|
| `otel.SetTracerProvider(tp)` | 全局注册，业务代码 `otel.Tracer("xxx")` 才能拿到 |
| `tp.Tracer("otel-demo/tracer_test")` | 创建 Tracer，参数是 instrumentation scope |
| `tp.Shutdown(ctx)` | flush + 释放，**不调会丢 span** |

### 3.2 父子 Span

```go
func TestTracer(t *testing.T) {
    tracer, shutdown := initTracer(t)
    defer shutdown()

    // 父 span
    ctx, parent := tracer.Start(context.Background(), "parent-op")
    parent.SetAttributes(attribute.String("layer", "web"))

    // 子 span：透传 ctx，自动挂到 parent 下
    func(ctx context.Context) {
        _, child := tracer.Start(ctx, "child-op")
        defer child.End()
        child.SetAttributes(attribute.String("layer", "service"))
        time.Sleep(10 * time.Millisecond)
    }(ctx)

    parent.End()

    assert.True(t, parent.SpanContext().IsValid())
    assert.True(t, parent.SpanContext().TraceID().IsValid())
}
```

**关键点**：
- `tracer.Start` 返回的 ctx 必须接住，并向下传递
- 闭包内传 `ctx` 而不是 `context.Background()`，否则 child 会变成新的 root span
- `parent.End()` 不会自动 End 子 span，**每个 Start 都要配 End**

### 3.3 Attributes / Events / Status

```go
func TestSpanAttributes(t *testing.T) {
    tracer, shutdown := initTracer(t)
    defer shutdown()

    _, span := tracer.Start(context.Background(), "http-request")
    defer span.End()

    // 属性
    span.SetAttributes(
        attribute.String("http.method", "POST"),
        attribute.String("http.route", "/users/:id"),
        attribute.Int("http.status_code", 200),
        attribute.Int64("user.id", 1024),
    )

    // 事件
    span.AddEvent("cache.miss", trace.WithAttributes(
        attribute.String("key", "user:1024"),
    ))
    span.AddEvent("db.query.done")

    // 状态
    span.SetStatus(codes.Ok, "")

    assert.True(t, span.SpanContext().IsValid())
}
```

## 四、运行 stdout 版

```bash
cd C:/Go/work/sandbox/opentelemetry
go test -v -run TestTracer
```

输出片段（节选）：

```json
{
    "Name": "child-op",
    "SpanContext": {
        "TraceID": "7c2e4dd1f8a04c8fae8b5e7b7c0b6b3a",
        "SpanID": "1a2b3c4d5e6f7080"
    },
    "Parent": {
        "TraceID": "7c2e4dd1f8a04c8fae8b5e7b7c0b6b3a",
        "SpanID": "0a1b2c3d4e5f6070"
    },
    "StartTime": "2026-04-18T14:22:01.123Z",
    "EndTime":   "2026-04-18T14:22:01.135Z",
    "Attributes": [{"Key": "layer", "Value": {"Type": "STRING", "Value": "service"}}],
    ...
    "Resource": [
        {"Key": "service.name", "Value": {"Type": "STRING", "Value": "otel-demo"}},
        ...
    ]
}
```

**怎么看输出**：
- `TraceID` 父子相同（都是 `7c2e...`），说明在同一条 trace
- 子 span 的 `Parent.SpanID` 等于父 span 的 `SpanID`，证明挂上了
- `Resource.service.name` = `otel-demo`，是 init 时塞的

## 五、Zipkin 版

### 5.1 启动 Zipkin

```bash
docker run -d --name zipkin -p 9411:9411 openzipkin/zipkin
# 浏览器访问 http://localhost:9411
```

### 5.2 跑测试

```bash
cd C:/Go/work/sandbox/opentelemetry
go test -v -run TestZipkin
```

测试代码 `zipkin_test.go` 与 stdout 版**核心差异**：

```go
exporter, _ := zipkin.New("http://localhost:9411/api/v2/spans")

tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter,                      // 改用 Batch
        sdktrace.WithBatchTimeout(2*time.Second),
    ),
    ...
)
```

只有 exporter 和 processor 换了，业务代码完全一样——**这就是 OTel 厂商中立的价值**。

### 5.3 在 UI 查看

1. 浏览器打开 http://localhost:9411
2. Service Name 选 `otel-demo-zipkin`
3. 点 "Run Query"
4. 点开任意一条 trace，能看到火焰图：父子 span、各自耗时、attribute、event

`TestZipkinErrorSpan` 产生的 span 会被标红（因为 `SetStatus(codes.Error, ...)`）。

### 5.4 看不到数据怎么办

- 测试日志看 `trace_id=...`，把它复制到 Zipkin "TraceID" 输入框直接搜
- 确认测试 PASS 没 SKIP（没起 Zipkin 时会自动 skip）
- 看容器日志：`docker logs zipkin`
- BatchSpanProcessor 异步发送，`tp.Shutdown(ctx)` 必须等到 flush 完成；测试代码已用 5s timeout

## 六、常见错误

| 现象 | 原因 | 解决 |
|------|------|------|
| 子 span 变成独立 root | 没透传 ctx 或传了 `context.Background()` | 全程透传 `tracer.Start` 返回的 ctx |
| span 没出现 | 忘了 `span.End()` 或 Provider 没 Shutdown | 检查 defer，加 Shutdown |
| Zipkin UI 搜不到 | exporter 地址错、Zipkin 没起、Batch 还没 flush | 确认地址、curl `localhost:9411/health`、加 Shutdown |
| `OTel ERROR: max queue size reached` | 后端慢，BatchProcessor 队列满 | 调大 `WithMaxQueueSize` 或排查后端 |
| schema URL 不一致警告 | 多个 semconv 版本混用 | 统一 semconv 版本号 |

## 七、下一步

- 看 `04-exporters.md` 选生产用什么 exporter
- 看 `05-integration.md` 把 OTel 接进 webook（gin / gorm / redis 中间件）
