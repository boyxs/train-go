# 最佳实践与踩坑清单

## 一、Span 命名规范

`tracer.Start(ctx, "<name>")` 的 name **不要包含动态值**（user_id、文章 id 等），否则后端 UI 按 name 聚合时基数爆炸。

| 场景 | ❌ 错 | ✅ 对 |
|------|------|------|
| HTTP handler | `GET /article/12345` | `HTTP GET /article/:id` |
| 数据库 | `SELECT * FROM users WHERE id=1024` | `MySQL SELECT users` |
| Service 方法 | `Publish article 12345` | `ArticleService.Publish` |
| 缓存 | `GET user:1024` | `Redis GET` |

**模式**：`{协议/库} {操作} {资源类型}` 或 `{Service}.{Method}`。动态信息放 `attribute`。

## 二、Attribute 规范

### 用 semconv 标准键名

```go
// ❌ 自创键名，后端不认
attribute.String("method", "POST")
attribute.String("path", "/users")

// ✅ semconv 标准
semconv.HTTPRequestMethodKey.String("POST"),
semconv.HTTPRoute("/users/:id"),
```

业务自定义键名加前缀避免冲突：`webook.article.id`、`webook.user.tier`。

### 别把大对象/PII 塞进去

| 类型 | ❌ | ✅ |
|------|----|----|
| 密码 / token | `attribute.String("password", pwd)` | 不写 |
| 整个请求体 | `attribute.String("body", string(b))` | 写关键字段或大小 |
| 结果列表 | `attribute.String("items", json.Marshal(list))` | `attribute.Int("items.count", len(list))` |
| 完整 SQL 参数 | `WHERE id=1024 AND email='a@b.com'` | GORM 用 `WithoutQueryVariables()` |

### 数值就用数值类型

```go
// ❌ 全部 String，UI 没法做数值过滤/排序
attribute.String("status_code", "200")
attribute.String("size", "12345")

// ✅
attribute.Int("http.status_code", 200)
attribute.Int64("response.size_bytes", 12345)
```

## 三、Span 生命周期

### 必须 End

```go
ctx, span := tracer.Start(ctx, "op")
defer span.End()                                  // 必备
```

漏 `End` 的后果：
- BatchSpanProcessor 不会导出未 End 的 span（**整条 trace 在 UI 看不到**）
- 内存泄漏（span 对象一直挂着）

### 错误处理三连

```go
if err != nil {
    span.RecordError(err)                         // 写入异常事件
    span.SetStatus(codes.Error, err.Error())      // 状态标错（UI 标红）
    return err                                    // 业务上抛
}
```

**`RecordError` 不会自动 SetStatus**，必须显式调。

### Context 必须透传

```go
// ❌ 子 span 断链，变成新 root
go func() {
    _, child := tracer.Start(context.Background(), "async-task")
    ...
}()

// ✅ 异步保留 trace
parentCtx := ctx
go func() {
    _, child := tracer.Start(parentCtx, "async-task")
    ...
}()
```

**注意**：父 span 可能在子 goroutine 还没执行完就 End 了，UI 上子 span 会比父 span 长。可接受，但要意识到。

## 四、采样策略

### 生产配置

```go
sdktrace.WithSampler(
    sdktrace.ParentBased(
        sdktrace.TraceIDRatioBased(0.1),          // 10% 采样
    ),
)
```

| 服务 QPS | 推荐采样率 |
|----------|-----------|
| < 100 | 1.0（全采） |
| 100 ~ 1k | 0.1 ~ 0.5 |
| 1k ~ 10k | 0.01 ~ 0.1 |
| > 10k | 0.001 + 尾部采样（错误/慢请求全留） |

### 错误/慢请求全留怎么做

头部采样 + 业务侧覆盖：

```go
// 默认 10% 采，但显式标记重要 trace
ctx, span := tracer.Start(ctx, "PaymentService.Pay")
// 关键交易：把当前 span 标 sampled=true（仅 ParentBased 下游遵守）
```

**真正完整的"错误/慢全留"必须在 OTel Collector 配 `tail_sampling` processor**，应用侧做不到（应用决策时还不知道是不是慢/错）。

### 不采样不等于无开销

即使 Sampler 决策不采，`tracer.Start` 仍然会创建 SpanContext（生成 TraceID/SpanID），只是不调用 Processor。开销很小（几 ns），但极致性能场景要意识到。

## 五、性能开销实测

OTel SDK 在典型场景的开销（参考社区 benchmark + 我们的经验值）：

| 操作 | 开销（约） |
|------|-----------|
| `tracer.Start` + `span.End`（不采样） | < 100 ns |
| `tracer.Start` + `span.End`（采样，进 BatchProcessor） | 1 ~ 3 μs |
| `SetAttributes`（5 个属性） | 200 ~ 500 ns |
| `AddEvent` | 200 ~ 500 ns |
| 内存：每个 span 约 1 ~ 2 KB |

**省心做法**：
- 关键路径 span 数控制在 10 个内
- 不要在循环里 start span（"每次循环一个 span" 几乎没用）
- BatchSpanProcessor 队列 `WithMaxQueueSize(2048)` 默认够用，QPS 高的服务调到 8192

## 六、调试与排查

### Span 没出现

1. 全局 Provider 注册了吗？`otel.SetTracerProvider(tp)`
2. 拿 Tracer 用的是 `otel.Tracer(...)` 还是别的 Provider？
3. `span.End()` 调了吗？
4. `tp.Shutdown(ctx)` 调了吗？BatchProcessor 队列里的 span 没 flush
5. Sampler 是不是设成 NeverSample
6. Exporter 地址对不对？telnet 后端端口
7. `otel.SetErrorHandler` 接一下，看 SDK 是否报错

### Trace 断成两截

1. 漏传 ctx：搜 `context.Background()` / `context.TODO()`
2. 跨进程没设 propagator：`otel.SetTextMapPropagator(...)`
3. 中间件顺序：otelgin 必须在自定义中间件**之前**注册
4. goroutine 没传 parent ctx

### Trace 数据量爆炸

1. Span name 含动态值（user_id、url path）→ 改成模板
2. 采样率太高 → 调 Sampler
3. 业务层手动埋点过多 → 砍掉低价值 span

## 七、利与弊总结

### OTel 带来什么

✅ 跨服务调用链可视化（异步消息也能串）
✅ 精准定位单次请求的慢点
✅ 错误根因分析（哪一层先报错）
✅ 厂商中立，未来切换后端零改动
✅ 与 Metrics/Logs 通过 trace_id 联动

### OTel 不擅长什么 / 代价

❌ 大盘聚合（QPS / 错误率）→ 用 Prometheus
❌ 海量明细日志检索 → 用 ELK / Loki
❌ 全采样高 QPS → 必须采样
❌ 增加部署复杂度（Collector / Backend / Storage）
❌ 学习曲线（团队成员都要懂 trace 模型）
❌ 二进制变大、启动慢一点

## 八、踩坑清单（按出现频率排序）

| # | 坑 | 表现 | 修复 |
|---|----|------|------|
| 1 | 漏 `defer span.End()` | UI 看不到 span，进程内存涨 | 静态检查 / Code review |
| 2 | 没透传 ctx | trace 断链 | grep `context.Background\|context.TODO` |
| 3 | 没 `tp.Shutdown` | 进程退出时丢最后一批 span | main 加 defer |
| 4 | 没设 propagator | 跨服务 trace 断 | `otel.SetTextMapPropagator(...)` |
| 5 | semconv 版本混用 | 启动 warning | 统一版本 |
| 6 | OTel 各包版本不一致 | 编译/运行报错 | go.mod 全部锁同 minor 版本 |
| 7 | Span name 含动态值 | UI 卡死 / 后端拒收 | name 用模板，动态值进 attribute |
| 8 | 把 PII / 大对象塞 attribute | 安全 + 存储成本 | 只放必要字段或 size |
| 9 | 全采样 + 高 QPS | 后端撑爆 / 应用 GC 飙升 | Sampler 调到 0.01 + 尾部采样 |
| 10 | `SetStatus(Ok)` 后被 `RecordError` 覆盖逻辑搞混 | 状态判断错 | RecordError + SetStatus(Error) 配套用 |
| 11 | BatchProcessor 队列满丢 span | `OTel ERROR: max queue size reached` | 调大队列 / 排查后端 |
| 12 | Trace UI 显示时间不对 | 时区 / 服务器时钟漂移 | NTP 同步、UI 看 UTC |

## 九、Code Review 检查点

接 OTel PR 看这些：
- [ ] 所有 `tracer.Start` 都有 `defer span.End()`
- [ ] 错误路径都 `RecordError + SetStatus(Error)`
- [ ] Span name 不含动态值
- [ ] Attribute 用 semconv 标准键名（自定义有前缀）
- [ ] 没把 PII / 大对象塞 attribute
- [ ] 中间件顺序：otelgin 在最前
- [ ] goroutine 透传 parent ctx
- [ ] 采样率与服务 QPS 匹配
