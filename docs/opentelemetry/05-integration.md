# 接入 webook

学习阶段在独立模块 `opentelemetry/` 验证，下一步把 OTel 接入 `webook/`。本章给出**集成蓝图**，具体落地代码等正式扩展时再写。

## 一、整体接入策略

```
webook/
├── ioc/
│   └── otel.go                # ① 初始化 TracerProvider，wire 注入
├── internal/web/middleware/
│   └── tracing.go             # ② Gin 中间件（用 otelgin）
├── internal/repository/dao/
│   └── article.go             # ③ GORM 用 otelgorm 插件，自动给 SQL 打 span
├── pkg/
│   ├── redisx/                # ④ Redis 用 otelredis（go-redis 自带）
│   └── kafkax/                # ⑤ Kafka 用 otelsarama 包装
└── cmd/main.go
```

**原则**：
- 业务代码尽量**不感知 OTel**，靠中间件和插件自动埋点
- 业务层只做"补充信息"：`span.SetAttributes(attribute.Int64("article.id", id))`
- 一个进程一个 TracerProvider，全局注册一次

## 二、初始化 TracerProvider（ioc/otel.go）

```go
package ioc

import (
    "context"
    "time"

    "github.com/spf13/viper"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitOTel() func(context.Context) error {
    type Config struct {
        Endpoint    string  `yaml:"endpoint"`
        ServiceName string  `yaml:"serviceName"`
        Env         string  `yaml:"env"`
        SampleRatio float64 `yaml:"sampleRatio"`
    }
    var cfg Config
    if err := viper.UnmarshalKey("otel", &cfg); err != nil {
        panic(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint(cfg.Endpoint),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        panic(err)
    }

    res, err := resource.Merge(
        resource.Default(),
        resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(cfg.ServiceName),
            semconv.DeploymentEnvironment(cfg.Env),
        ),
    )
    if err != nil {
        panic(err)
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter,
            sdktrace.WithBatchTimeout(5*time.Second),
            sdktrace.WithMaxExportBatchSize(512),
            sdktrace.WithMaxQueueSize(2048),
        ),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.ParentBased(
            sdktrace.TraceIDRatioBased(cfg.SampleRatio),
        )),
    )
    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

    // 返回 Shutdown 函数，main 里 defer
    return tp.Shutdown
}
```

**配置示例（config/dev.yaml）**：

```yaml
otel:
  endpoint: "localhost:4317"
  serviceName: "webook"
  env: "dev"
  sampleRatio: 1.0       # dev 全采样；prod 改 0.1
```

**main.go 收尾**：

```go
shutdown := ioc.InitOTel()
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = shutdown(ctx)
}()
```

## 三、Gin 中间件（otelgin）

```go
import "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

server := gin.Default()
server.Use(otelgin.Middleware("webook"))         // 服务名作为 instrumentation scope
```

**自动做的事**：
- 从 HTTP header 提取 `traceparent`，作为 parent span context（跨服务串联）
- 创建 root span（Kind=Server），命名 `HTTP {method} {route_pattern}`
- 自动设属性：`http.method`、`http.route`、`http.status_code`、`http.user_agent`
- 异常 panic 自动 RecordError + SetStatus(Error)

**与 Prometheus 中间件兼容**：两者都用 `c.FullPath()` 拿路由 pattern，互不冲突。`otelgin` 在前还是 `prometheus` 中间件在前都可以，建议 **`otelgin` 在前**，让 prometheus 中间件能拿到 trace context（未来加 exemplar）。

## 四、GORM（otelgorm）

```go
import "gorm.io/plugin/opentelemetry/tracing"

db, _ := gorm.Open(...)
db.Use(tracing.NewPlugin())                       // 自动给所有 SQL 打 span
```

**自动做的事**：
- 每条 SQL 一个 span（Kind=Client）
- 设属性：`db.system`、`db.statement`、`db.operation`、`db.sql.table`
- 错误自动 RecordError

**注意**：`db.statement` 是完整 SQL，可能含敏感字段（密码 hash 等），生产配 `tracing.WithoutQueryVariables()` 隐藏参数值。

## 五、Redis（go-redis v9 自带）

```go
import "github.com/redis/go-redis/extra/redisotel/v9"

rdb := redis.NewClient(&redis.Options{...})
if err := redisotel.InstrumentTracing(rdb); err != nil {
    panic(err)
}
```

**自动做的事**：
- 每个 Redis 命令一个 span
- 设属性：`db.system="redis"`、`db.statement="GET user:1024"`

## 六、Kafka（otelsarama）

```go
import "go.opentelemetry.io/contrib/instrumentation/github.com/IBM/sarama/otelsarama"

// Producer
producer = otelsarama.WrapSyncProducer(cfg, producer)

// Consumer
handler := otelsarama.WrapConsumerGroupHandler(myHandler)
```

**自动做的事**：
- Producer：发消息时把 trace context **写入 Kafka header**
- Consumer：消费时从 header 提取，把消费 span 挂到 producer 的 trace 上
- 这样**异步消息也能串成完整调用链**（webook 的 reward / 积分 / 动态推送等异步流可视化）

## 七、业务层手动埋点

中间件覆盖了入口和出口，业务层关键节点可以手动加 span，让火焰图更细：

```go
func (s *ArticleService) Publish(ctx context.Context, art domain.Article) (int64, error) {
    ctx, span := otel.Tracer("webook/service/article").Start(ctx, "ArticleService.Publish",
        trace.WithAttributes(attribute.Int64("author.id", art.Author.Id)),
    )
    defer span.End()

    id, err := s.repo.Sync(ctx, art)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return 0, err
    }
    span.SetAttributes(attribute.Int64("article.id", id))
    return id, nil
}
```

**手动埋点的取舍**：
- ✅ 关键业务步骤（创建订单、发布文章、扣减库存）
- ✅ 复杂分支（限流命中、降级路径、缓存策略选择）
- ❌ 每个 helper 函数都加（信噪比低，trace 树太深）

## 八、跨服务传播

如果 webook 调用外部 HTTP 服务：

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

client := http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport),
}
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)                       // 自动注入 traceparent header
```

下游如果也用 OTel + W3C TraceContext propagator，trace 自动贯穿。

## 九、与 Prometheus 协作（exemplar）

Prometheus 2.30+ 支持 exemplar：在 Histogram 桶里挂 trace_id。Grafana 看图时能从高延迟点直接跳到对应 trace。

webook 现有 `pkg/ginx/middleware/prometheus`：

```go
// 改造点：观测 Histogram 时附带当前 span 的 TraceID
if span := trace.SpanFromContext(c.Request.Context()); span.SpanContext().IsValid() {
    histogram.(prometheus.ExemplarObserver).ObserveWithExemplar(
        duration,
        prometheus.Labels{"trace_id": span.SpanContext().TraceID().String()},
    )
}
```

需要把 `otelgin` 放在 `prometheus` 中间件**之前**，确保观测时 ctx 里已有 span。

## 十、接入顺序建议

1. **第一步**：`ioc/otel.go` + `otelgin` 中间件 → 看到 HTTP 入口的 span
2. **第二步**：`otelgorm` → SQL 也进 trace
3. **第三步**：`redisotel` → Redis 命令进 trace
4. **第四步**：业务层关键节点手动埋点（service 层）
5. **第五步**：`otelsarama` → 异步消息链路串起来
6. **第六步**：跨服务调用 `otelhttp`
7. **第七步**：Prometheus exemplar 联动

每一步独立可验证，不要一次接全。
