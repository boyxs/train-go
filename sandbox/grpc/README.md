# gRPC + etcd 服务注册/发现(设计 + demo)

> 本仓 gRPC 服务注册/发现的设计文档 + 可运行 demo。
> 生产实现:`webook/pkg/grpcx` + 各服务 `ioc`(§1–§6、§8–§10)。
> 本目录 `sandbox/grpc`:自包含可运行 demo,原生 / go-zero / kratos 三种(§7)。

## 1. 背景与目标

服务从单体 core 拆出 chat / migrator 后，chat 要 RPC 调 core 的 search / article / interaction。
写死 `coreAddr: "webook-core:8011"` 的问题：core 多副本/换 IP/扩缩容时客户端不感知。

目标：

- **服务端**注册自己到 etcd（带租约，宕了自动摘除）。
- **客户端**经 etcd resolver 动态发现实例，无需写死地址。
- etcd **既是配置中心**（viper remote）**又是注册中心**，复用同一集群。
- 通用能力沉淀到 `pkg/grpcx`，新增服务/下游**纯增量接入**。

## 2. 整体拓扑

```
                    ┌─────────────── etcd 集群 ───────────────┐
                    │  /webook-core         (配置中心 KV)        │
   配置热更 ◀───────│  /webook-chat                            │
                    │  service/webook-core/10.0.0.1:8011  ◀──┐ │
                    │  service/webook-core/10.0.0.2:8011     │ │ 注册(带 lease)
                    └────────────────┬───────────────────────┼─┘
                          resolver   │ watch prefix           │ KeepAlive(ttl/3)
                          发现        ▼ "service/webook-core/" │
   ┌──────────────┐   etcd:///service/webook-core    ┌────────┴────────┐
   │  webook-chat │ ─────────────────────────────▶   │   webook-core   │
   │ (gRPC client)│   GetUser / Search / ...          │  (gRPC server)  │
   └──────────────┘                                   └─────────────────┘
```

- 注册键：`service/<Name>/<host:port>`，绑 lease；KeepAlive 每 ttl/3 续租，进程挂 → lease 过期 → 键自动消失。
- 客户端 target `etcd:///service/<Name>`，etcd resolver watch `service/<Name>/` 前缀，实例增删实时感知。

## 3. 核心抽象 `pkg/grpcx`

两侧 API 对称（对齐标准库 `grpc.NewServer` / `grpc.NewClient`）：

| 维度 | Server 侧 | Client 侧 |
|------|-----------|-----------|
| 配置 | `ServerConfig{Port, Name, Host, TTL}` | `ClientConfig{Target, Secure, CAFile}` |
| 构造 | `NewServer(cfg, etcdCli, l, opts...) *Server` | `NewClient(etcdCli, cfg, opts...) (*grpc.ClientConn, func(), error)` |
| 横切 option | 调用方(ioc)经 `opts` 传 otel + 拦截器 | 同左 |
| 产物 | `*grpcx.Server`（自管注册） | `*grpc.ClientConn`（同 `grpc.NewClient`） |

**横切 option 由调用方（ioc）经 `opts ...` 显式传**，grpcx 不内置默认。otel 链路追踪（`otelgrpc` StatsHandler，otel.disabled 时 noop）和错误码拦截器在 `internal/ioc/grpc.go` / `chat/ioc/grpc.go` 各自传入；超时 / keepalive / 最大消息大小 / 额外拦截器同样走 `opts`。`NewServer` / `NewClient` 本身只管 grpc.Server 构造 / etcd resolver + 凭证。**代价**：没有"改一处全生效"的统一默认，新服务要记得把 otel + 拦截器一起传（漏传 = 没链路追踪 / 错误码不转）。

### 3.1 Server（`server.go`）

```go
type Server struct {
    *grpc.Server                 // 内嵌，调用方在 .Server 上注册 service
    Port   int
    Name   string                // 服务名 → 注册命名空间
    Host   string                // 广告 host；空则探测出口 IP
    TTL    int64                 // 租约 TTL(秒)，<=0 用 defaultLeaseTTL(30)
    Client *etcdv3.Client        // 外部注入，生命周期不归 Server
    L      logger.LoggerX
    // em / key / kaCancel 内部状态
}

func NewServer(cfg ServerConfig, client *etcdv3.Client, l logger.LoggerX, opts ...grpc.ServerOption) *Server

func (s *Server) Serve() error     // 按 Port 监听 + 启动（阻塞）
func (s *Server) Register() error  // 注册 etcd + 续租（中断自动重注册）
func (s *Server) Close() error     // 停续租 → 注销端点 → GracefulStop
```

- **职责分离**：Server 只「用」注入的 etcd client，不创建/不关闭它（client 由 `ioc.InitEtcdClient` 拥有、wire cleanup 统一关）。
- **Close 顺序**：停续租 → `DeleteEndpoint` 主动注销 → `GracefulStop`，返回首个错误；不碰 client。

### 3.2 Client（`client.go`）

```go
type ClientConfig struct {
    Target string // 解析目标，如 etcd:///service/webook-core
    Secure bool   // false→insecure（默认），true→TLS
    CAFile string // secure=true 时验签 CA；空用系统根证书
}

// 装 etcd resolver + 按 cfg 构造传输凭证(insecure/TLS)；otel、拦截器等正交 option 由调用方经 opts 传入
func NewClient(client *etcdv3.Client, cfg ClientConfig, opts ...grpc.DialOption) (*grpc.ClientConn, func(), error)
```

## 4. 配置约定

| 配置键 | 用途 | 示例 |
|--------|------|------|
| `etcd.endpoints` | etcd 集群地址列表（配置中心 + 注册中心共用） | `["http://webook-etcd:2379"]` |
| `grpc.server.port` | 本服务 gRPC 监听端口 | `8011` |
| `grpc.server.name` | 服务名（注册命名空间） | `webook-core` |
| `grpc.server.ttl` | 租约 TTL(秒) | `30` |
| `grpc.server.host` | 注册广告 host(k8s 填 POD_IP)；空则探测出口 IP | `""` |
| `grpc.client.<name>.target` | 下游 resolver target | `etcd:///service/webook-core` |
| `grpc.client.<name>.secure` | 下游传输:false→insecure(默认)，true→TLS | `false` |
| `grpc.client.<name>.caFile` | secure=true 时验签 CA;空用系统根证书 | `""` |

**约定优于配置**：客户端 `target` 缺省按 `etcd:///service/<name>` 推导，接新下游通常**零配置**，除非改 target 或开 TLS。

> **集群**：`etcd.endpoints` 是列表，etcd client 原生跨节点 failover；配置中心侧（`pkg/viperx.WatchRemote`）循环 `AddRemoteProvider` 逐个注册。现各填 1 节点，上集群时往列表加即可，代码不动。

## 5. 注册/发现数据流

### 5.1 注册（`Server.Register`）

```
NewManager(client, "service/<Name>")
  → Grant(ctx, ttl)                      申请租约（ttl<=0 → 30s）
  → AddEndpoint("service/<Name>/<host:port>", {Addr}, WithLease)   写端点，绑租约
      └─ 失败 → Revoke(lease) 回收租约，errors.Join 返回，不悬挂
  → KeepAlive(独立 ctx, lease)           etcd 每 ttl/3 自动续租
      └─ goroutine 消费续租响应；channel 关(etcd 抖动/lease 过期)→ 退避自动重注册；Close 时 cancel 退出
```

- `<host:port>` 的 host 取 `grpc.server.host`，空则 `netx.ExternalIp()`（拨 8.8.8.8 探出口 IP）。
- 续租用**独立 ctx**（非请求 ctx），生命周期绑 Server，`Close` 时取消 → 不泄漏 goroutine。

### 5.2 发现（`NewClient`）

```
resolver.NewBuilder(etcdClient)                    建 etcd resolver
  → grpc.NewClient("etcd:///service/<Name>",        target 的 path 即 watch 前缀
        WithResolvers(r), 凭证(按 cfg.Secure/CAFile), opts...)   // opts(otel/拦截器)由调用方传
  → 一条 ClientConn 复用给该服务的多个 typed client（HTTP/2 多路复用）
```

## 6. 集成用例

### 6.1 core 作为 server 注册自己

```go
// internal/ioc/grpc.go
func InitGRPCServer(searchSrv, articleSrv, intrSrv, client *etcdv3.Client, l logger.LoggerX) *grpcx.Server {
    var cfg grpcx.ServerConfig
    viper.UnmarshalKey("grpc.server", &cfg)        // {8011, webook-core, 30}
    srv := grpcx.NewServer(cfg, client, l,           // otel + 错误拦截器经 opts 显式传
        grpc.StatsHandler(otelgrpc.NewServerHandler()),
        grpc.UnaryInterceptor(interceptor.UnaryServerError()))
    searchv1.RegisterSearchServiceServer(srv.Server, searchSrv)
    articlev1.RegisterArticleReaderServiceServer(srv.Server, articleSrv)
    interactionv1.RegisterInteractionServiceServer(srv.Server, intrSrv)
    healthpb.RegisterHealthServer(srv.Server, health.NewServer())  // 健康检查
    return srv
}

// internal/main.go —— gRPC 后台注册+监听；main 等信号后优雅停机
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
go func() { app.GRPCServer.Register(); app.GRPCServer.Serve() }()
httpSrv := &http.Server{Addr: addr, Handler: app.Server}   // 用 http.Server 以便优雅关
go httpSrv.ListenAndServe()
<-ctx.Done()
app.GRPCServer.Close()    // 注销 etcd 端点 + GracefulStop
httpSrv.Shutdown(ctx10s)  // 等在途请求(最多 10s)
cleanup()                 // OTel flush 等
```

etcd client 经 `ioc.InitEtcdClient`（读 `etcd.endpoints`）注入，wire cleanup 统一关闭。

### 6.2 chat 作为 client 发现 core

```go
// chat/ioc/grpc.go
type CoreConn struct{ *grpc.ClientConn }   // ← 命名类型，见下「关键点」

func InitCoreConn(client *etcdv3.Client) (CoreConn, func(), error) {
    cfg, err := clientConfig("webook-core")          // target 缺省 + secure/caFile 读进 cfg
    if err != nil { return CoreConn{}, nil, err }
    conn, cleanup, err := grpcx.NewClient(client, cfg,   // 凭证由 cfg.Secure/CAFile 构造
        grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
        grpc.WithUnaryInterceptor(interceptor.UnaryClientError()))
    if err != nil { return CoreConn{}, nil, err }
    return CoreConn{conn}, cleanup, nil
}

func InitSearchClient(c CoreConn) searchv1.SearchServiceClient { return searchv1.NewSearchServiceClient(c) }
// ArticleReader / Interaction 同理，共用同一条 CoreConn
```

**关键点 — `CoreConn` 命名类型解决 wire 多 conn 冲突**：wire 按**类型**注入，若两个下游的 `InitXxxConn` 都返回裸 `*grpc.ClientConn` 会冲突。给每条 conn 一个独立类型（内嵌 `*grpc.ClientConn` → 自带 `ClientConnInterface` 方法），wire 即可区分。

## 7. 可运行 demo(本目录 `sandbox/grpc`)

用 `UserService` 把上面的注册/发现落成可跑代码,并对比**三种实现**:原生 gRPC+etcd、go-zero、kratos。`sandbox/grpc` 是独立 module(自带 go.mod)。

### 目录

| 路径 | 说明 |
|------|------|
| `proto/user/v1/` · `gen/user/v1/` | UserService 契约与 protoc 生成（`make gen`） |
| `server/user_server.go` | `MemoryUserServer`:内存实现,三种 demo 共用（含一行注释掉的「模拟超时」`time.Sleep` 开关） |
| `registry/registry.go` | `Registry` 接口 + `ServiceInstance{Name,Addr,Weight,Metadata}` |
| `registry/etcd.go` | `EtcdRegistry`:原生 etcd 注册器（多服务、weight/metadata、`WithTTL`、失败回收租约）—— `pkg/grpcx.Server` 注册逻辑的教学拆解版 |
| `registry/registry_test.go` | `TestRegistryRoundTrip`:自动化往返（无 etcd 自动 skip） |
| `registry/{etcd,gozero,kratos}_demo_test.go` | 三种框架的手动 demo（见下） |

### 三种实现对比

| Demo | 注册/发现机制 |
|------|--------------|
| `etcd_demo_test.go`（原生） | 手动 `endpoints.Manager` + `Grant`/`KeepAlive` 续租;client 走 etcd resolver（即 §3~§5 的裸实现） |
| `gozero_demo_test.go` | go-zero:`zrpc.RpcServerConf/RpcClientConf` + `discov.EtcdConf`，框架托管 |
| `kratos_demo_test.go` | kratos:`kratos.Registrar(etcd.New)` + `grpc.WithDiscovery`，注册键带 `/microservices/` 前缀 |

> 三者注册键编码不同（原生 `service/<name>/…`、go-zero `user.rpc/…`、kratos `/microservices/user.rpc/…`），**不跨框架互通**，各自 server+client 配对自洽。

### 前置:本地 etcd（:2379）

```bash
docker run -d --name etcd -p 2379:2379 \
  -e ALLOW_NONE_AUTHENTICATION=yes \
  -e ETCD_ADVERTISE_CLIENT_URLS=http://0.0.0.0:2379 \
  bitnami/etcd:latest
```

### 怎么跑（命令在 `sandbox/grpc/` 下执行）

```bash
make gen     # 改了 .proto 才需要
make test    # 全量;无 etcd 手动开关时,注册/发现用例安全 SKIP,不会挂

# 自动化往返(注册 → resolver 发现 → 调用 → 注销),无 etcd 自动 skip
go test ./registry/ -run TestRegistryRoundTrip -v
```

各框架 demo 的 `TestServer` / `TestClient` 是**手动**用例（Server 阻塞），裸 `go test` 一律 SKIP。真跑需 `ETCD_MANUAL=1` + 两个终端，以 go-zero 为例：

```bash
# 终端1:起 server 注册到 etcd
ETCD_MANUAL=1 go test ./registry/ -run 'TestGoZero/TestServer' -v
# 终端2:发现并调用
ETCD_MANUAL=1 go test ./registry/ -run 'TestGoZero/TestClient' -v
```

`TestGoZero` 换成 `TestKratos` / `TestEtcd` 跑另两种。Windows PowerShell 用 `$env:ETCD_MANUAL="1";` 前缀。

### demo 里的两个坑

- **go-zero `Timeout` 不生效**:手写 `RpcClientConf` 字面量不经 `conf.Load`，`Middlewares` 的 `default=true` 不会被填（零值=全关）→ 必须显式 `Middlewares: zrpc.ClientMiddlewaresConf{Timeout: true}`，否则 TimeoutInterceptor 根本不装、`Timeout` 形同虚设。
- **模拟超时**:`server/user_server.go` 那行注释掉的 `time.Sleep` 取消注释后，client（Timeout 短于它）即可触发 `DeadlineExceeded`;平时保持注释，免得拖慢共用 server。

## 8. 扩展指南

| 扩展场景 | 做法 | 动 grpcx 吗 |
|----------|------|------------|
| 新增 server 服务注册自己 | 镜像 `internal/ioc`（`InitEtcdClient` + `InitGRPCServer` 用 `NewServer`）+ main `Register/Serve/Close`；配 `grpc.server` | ❌ 不动 |
| 某 client 调新下游 | 加 `type XxxConn struct{ *grpc.ClientConn }` + `InitXxxConn`(用 `NewClient`) + typed client + wire 一行；配置可省（target 约定兜底） | ❌ 不动 |
| 同一 client 调多个下游 | 每条 conn 用独立命名类型（wire 按类型注入，避免 `*grpc.ClientConn` 冲突） | ❌ 不动 |
| 改某下游 dial option（超时/重试/拦截器） | 在对应 `InitXxxConn` 的 `grpcx.NewClient(..., opts...)` 传 | 仅该下游 |

加新下游示例（chat 再调 webook-feed）：

```go
type FeedConn struct{ *grpc.ClientConn }
func InitFeedConn(cli *etcdv3.Client) (FeedConn, func(), error) {
    cfg, err := clientConfig("webook-feed")          // target 自动 = etcd:///service/webook-feed
    if err != nil { return FeedConn{}, nil, err }
    conn, cleanup, err := grpcx.NewClient(cli, cfg)
    if err != nil { return FeedConn{}, nil, err }
    return FeedConn{conn}, cleanup, nil
}
func InitFeedClient(c FeedConn) feedv1.FeedServiceClient { return feedv1.NewFeedServiceClient(c) }
// + wire.Build 加 InitFeedConn, InitFeedClient；core 一行不动
```

## 9. 风险与权衡

| 项 | 说明 / 取舍 |
|----|------------|
| etcd 单点 vs 集群 | `etcd.endpoints` 已支持多节点；当前各环境填 1 节点，上集群往列表加即可 |
| 配置中心降级 | `etcd.endpoints` 未配置 / 不可达时 viper remote 告警 + 降级本地 yaml（不 panic、不静默连 localhost）；服务发现侧（`InitEtcdClient`）相反，`endpoints` 未配置即 fail-fast |
| 优雅停机 | main 监听 SIGINT/SIGTERM → `Server.Close()` 注销端点 + GracefulStop + `http.Shutdown`，不留过期注册 |
| 健康检查 | server 注册 `grpc_health_v1`（`health.NewServer()`），供 k8s liveness/readiness、LB 探测 |
| 续租中断恢复 | KeepAlive channel 关(etcd 抖动/lease 过期)→ 退避自动重注册并告警；`Close` 取消则退出，不泄漏 |
| 注册地址 | 优先 `grpc.server.host`（k8s 填 POD_IP）；空才 fallback `netx.ExternalIp()`（拨 8.8.8.8，无外网/多网卡不可靠） |
| 传输安全 | ioc 按 yaml `secure`/`caFile` 构造凭证经 `opts` 传：false→insecure、true→TLS(caFile 验签)；grpcx 不预设。mTLS 再加客户端 cert/key 即可 |
| 配置校验 | `Register` 校验 `Name`/`Port`、`InitEtcdClient` 校验 `etcd.endpoints` 非空 → 非法即 fail-fast，不塞静默默认值 |
| `Register` 失败回收 | `AddEndpoint`/`KeepAlive` 失败 `Revoke` 租约 + `errors.Join`，不留悬挂租约 |
| etcd 版本兼容 | 锁 `etcd client/v3 v3.6.12`：旧 alpha 的 `naming/resolver` 与新 grpc-go 不兼容会编译不过 |

## 10. 关键文件索引

| 关注点 | 文件 |
|--------|------|
| Server 注册/续租/注销 | `webook/pkg/grpcx/server.go` |
| Client 发现拨号 | `webook/pkg/grpcx/client.go` |
| 错误码拦截器（server/client 双向） | `webook/pkg/grpcx/interceptor/error.go` |
| etcd client provider | `webook/{internal,chat}/ioc/etcd.go` |
| 配置加载（本地 yaml + etcd 远程 watch） | `webook/pkg/viperx/viperx.go` |
| core 组装 server | `webook/internal/ioc/grpc.go` + `internal/main.go` |
| chat 组装 client | `webook/chat/ioc/grpc.go` + `chat/wire.go` |
| 出口 IP | `webook/pkg/netx/ip.go` |
| 可运行 demo | 本目录 `registry/`（原生 / go-zero / kratos 三种,见 §7） |
