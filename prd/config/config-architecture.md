# 配置架构设计（标准版）

> 状态：重定稿（2026-07-03）｜ 模块：config ｜ 关联：`webook/CLAUDE.md`「环境说明」、`pkg/viperx`
>
> `<svc>/config/` 只放各环境 yaml；配置的**类型真相源**是各段的 Go struct（各归其位，见 §6）。本文档为字典与设计依据。
> 新增配置项流程：先在此登记（标注 **[必填]** / **[默认 X]**）→ 落段 struct（`mapstructure` tag，各归其位）→ 落 yaml（仅必填/环境差异键）。
>
> 本架构只用 **viper + 标准 Go 类型化 struct**，不引入任何框架式 config loader（proto schema / 内聚配置框架），也不设中央 Bootstrap 与校验层。设计以 grpc-go 语义与本项目实际需求自证，不对标外部框架。

## 1. 背景与目标

现状 6 个服务 × 5 份环境 yaml，存在五类问题：

| 问题 | 现状举例 |
|------|---------|
| 监听面拆散 | `http.addr` 与 `grpc.server.port` 顶层并列，一个用 addr 一个用 port |
| 时间值无单位 | `dialTimeout: 10  # 秒`、`metadataRetryBackoff: 250  # 毫秒`，秒/毫秒混用靠注释背书 |
| 无超时治理 | HTTP server、gRPC server / client 均无 timeout 配置项 |
| 配置错误暴露晚 | 键拼错 / 缺必填 = 静默零值，运行时才炸且离根因远 |
| 密钥进 git | 真实 LLM / embedding apiKey 明文提交在 yaml 里 |

目标：用**标准类型化配置**统一域划分、键命名、时间格式、密钥管理、注释规范——

- **身份/接线键显式声明**：wire Provider + yaml 段一一对应，代码绝不派生兜底；缺失/拼错 → 消费点自然失败（不设校验层，见 §6）；
- **最优调参默认值在代码作兜底**（如 `ttl=30s`、`watchdog=ttl/3`）；yaml 显式写出配置键、可逐环境改，代码默认仅 fallback；
- 类型层强约束（强类型字段、无弱类型容器、无裸 JSON）；
- **超时治理一并落地**（不拆期）：HTTP / gRPC server + client 超时，流式 / 出站 LLM 豁免；
- 给出存量迁移映射与实施任务（一次性，不分期）。

## 2. 设计原则

1. **域分组**：`server`（本服务监听面）/ `client`（下游依赖面）/ `data`（存储与中间件）三大核心域 + 平台段（`etcd` / `logger` / `otel`）+ 业务段顶层平铺（`llm` / `sensitive` / `migrator` …）。按**职责与生命周期**聚合，不按类型散铺顶层。
2. **叶子键 snake_case**：yaml 生态（K8s / Prometheus / OTel Collector）通用惯例；env 映射干净（`access_log.max_req_len` → `ACCESS_LOG_MAX_REQ_LEN`）。Go struct 用 `mapstructure:"snake_key"` tag 承接。
3. **时间值一律 duration 字符串**（`3s` / `250ms` / `1m`）：viper 挂 `StringToTimeDurationHookFunc`，Unmarshal 直接得 `time.Duration`；禁止裸整数 + 单位注释。
4. **地址统一 `addr`**（`":8011"` / `"host:port"` 形式），废除 `port` 整数键。
5. **就地读取，无中央装配与校验**：`<svc>/config/` 只放 yaml；每个 ioc Provider `viper.UnmarshalKey("<段>", &cfg)` 读自己那段直接用（沿用现有 `internal/ioc/otel.go` 模式），**无中央 Bootstrap struct、无 MustLoad、无 Validate 层**。段类型各归其位（§6）。配置错误在**消费点自然暴露**（空 `target` → dial 失败、空 `name` → `Register` 报错、坏 `dsn` → 连库失败），不在加载期集中拦截。
6. **身份/接线键显式声明，禁止「过度兼容」的构建**：身份/接线配置（`addr` / `dsn` / `target` / `balancer` / 服务名 / `endpoints`）由 **wire Provider + yaml 段一一对应**显式声明，代码**绝不派生、绝不猜测、绝不静默兜底**缺失值。反例——老 `grpcClientConfig` 在 target 缺失时按 `etcd:///service/<name>` 推导：段名拼错照样"能跑"，问题被推迟到 RPC 且症状远离根因。本架构不写这种"替业务补全的构建方法"；缺了就是零值 → 消费点自然失败，**而非被派生值掩盖**（这才是"显式"的意义，不靠校验层兜）。仅允许「功能开关式缺省」（段缺失 = 功能关闭，如 `etcd.path` 不配 = 不接热更），且必须登记在本文档。
7. **最优调参默认值就地兜底**：性能调参键（`ttl` / `watchdog` / `weight` / `retry` / `keep_alive` / `msg size` / 各类超时退避）的最优值**写在消费它的代码里**（`const` + `if <=0` 兜底，或 functional-option 默认），作 yaml 缺失 / 写 0 时的兜底 fallback（yaml 正常显式写出这些键、可改），与消费代码同版本演进。范本已在仓内：`grpcx.defaultLeaseTTL=30`（`server.go:20`）、`redislockx.applyOptions{watchdogInterval: ttl/3, retryInterval: 100ms}`（`options.go:55`）。**不在 config 层再写集中的默认应用方法**（无 `applyDefaults()`、无远离消费点的 `viper.SetDefault` 表）。与原则 6 的分界线见 §4。
8. **密钥不进 git**：外部真实凭据（LLM / embedding apiKey）一律 `${ENV}` 占位注入；本机 docker 自建中间件密码按环境处理（见 §9）。
9. **不落死配置**：代码没消费的键不进 yaml。有推荐值的调参键在 yaml 显式写出、可改，代码默认仅兜底；仅功能开关式缺省与纯 pkg 内部调参可省（见 §4）。

## 3. 类型约束

配置的类型真相源是各段的 Go struct（各归其位，见 §6），约束在**类型层与解码期**表达，**不设运行时校验层**：

1. **强类型字段，禁弱类型容器**：每段是纯 Go struct + `mapstructure` tag，字段用精确类型——时间 `time.Duration`、地址 `string`、开关 `bool`、列表 `[]string`、数值 `int` / `float64`。**禁止** `map[string]any` / `interface{}` / 裸 JSON 字符串承接结构化配置（内容级拼错静默不生效）。
2. **struct 全导出**：段类型 + 字段大写（`grpcx.ServerConfig` / `grpcx.ClientConfig` / ioc 内联的 `Kafka` 等），符合仓内「所有 struct 必须导出」规则。
3. **未配置 = 类型零值，语义分流**：调参键零值（`0` / `""` / `nil`）→ 消费点**就地兜底**为最优默认（§4）；身份/接线键零值 → 消费点**自然失败**（空 target→dial 失败、坏 dsn→连库失败、空 name→Register 报错）。
4. **不设校验层**（用户决策：彻底不校验）：不写 `Bootstrap.Validate` / 段 `Validate()` / 必填检查 / 枚举白名单 / `ErrorUnused`。取值合法性交**消费库自然兜住**——`logger.level` 交 `zapcore.ParseLevel`、gRPC code 交 grpc 解析，非法即在初始化时自然报错。**代价**：配置错误落到首次消费才暴露、症状可能离根因稍远（见 §12），换来的是零校验样板。

## 4. 默认值策略：必填 vs 调参

一条分界线贯穿全部配置键：

| 类别 | 键例 | yaml 写法 | 缺失时 | 默认值落点 |
|------|------|----------|--------|-----------|
| **身份 / 接线**（必填） | `server.grpc.addr`、`data.mysql.dsn`、`client.grpc.<svc>.target` / `balancer`、`server.grpc.name`、`etcd.endpoints`、`data.redis.addr` | 全环境显式写 | 消费点自然失败（dial/连库/Register 报错） | 无默认 |
| **调参**（可选，进 yaml） | `server.grpc.ttl` / `weight`、`client.grpc.<svc>.retry` / `keep_alive` / `max_*_msg_size`、`data.kafka.*_timeout`、`llm.providers[].timeout` / `max_tokens` | 省略（偏离默认才写） | 用代码默认（"用推荐值"） | **代码就地兜底** |
| **纯代码调参**（不进 yaml） | `redislockx` 的 `watchdog` / `retryInterval` | 不出现 | 用代码默认 | **代码就地兜底** |

**判据**：这个键填错 / 缺了，是"配置写错了"还是"没指定，用最优"？错了 → 必填（缺则消费点自然失败，原则 6）；没指定就用最优 → 调参、兜底（原则 7）。

### 兜底机制：就地，不集中

默认值写在**消费它的代码**里，紧挨使用点、与消费逻辑同版本。仓内两种既有范本，新代码沿用，不另起集中式默认框架：

```go
// 范本一：const + 消费点兜底（pkg/grpcx/server.go）
const defaultLeaseTTL int64 = 30
ttl := s.TTL
if ttl <= 0 { // yaml 省略 ttl → 解到 0 → 用 30s
    ttl = defaultLeaseTTL
}

// 范本二：functional-option 默认（pkg/redislockx/options.go）
cfg := &lockConfig{
    retryInterval:    100 * time.Millisecond,
    watchdogInterval: ttl / 3, // 最优默认；调用方只在偏离时传 WithWatchdog
}
for _, o := range opts {
    o(cfg)
}
```

**不做的事**（对应"不写一个方法过度兼容配置的构建"）：

- 不在 config 层写集中的 `applyDefaults()` / `SetDefaults()` 批量补零值字段
- 不用 `viper.SetDefault` 列一张远离消费点的默认表
- 不写替业务派生 / 猜测缺失身份键的"配置构建方法"

### yaml 写法：显式配置 + 代码兜底

- **配置键显式写出**（身份键 + 有推荐值的调参键都写，含 `ttl` / `weight` / `host` / `timeout` / kafka 超时 / llm timeout 等）→ operator 一眼可见、可逐环境改（「万一要改呢」）；5 份 yaml 逐环境 diff 可审计；
- **代码默认只作兜底 fallback**：yaml 缺失 / 写 0 时用 `const` / functional-option 默认（§兜底机制），不承载真相——真相在 yaml；
- **仅两类可省**：①功能开关式缺省（`keep_alive` / `retry` / `health_check` / `max_*_msg_size`，不写 = 关闭 / 库默认，启用才写，原则 6）；②纯 pkg 内部调参（`watchdog` / `retryInterval`，不经 yaml，functional-option 默认）。

字典中每个键标注 **[必填]** 或 **[默认 X]**，X 即代码兜底值——见 §4 末「调参键默认值字典」。

## 5. 目标 schema

以 webook-core prod 为例（配置键显式写出、可逐环境改；代码默认仅兜底；功能开关段不写=关闭）：

```yaml
# webook-core prod 配置。schema 与默认值见 prd/config/config-architecture.md
# 配置键显式写出(可逐环境改)；代码默认仅在 yaml 缺失/写 0 时兜底

server:
  http:
    addr: ":8010"
    timeout: 15s              # 请求超时；SSE 路径豁免；<=0 兜底 15s
    access_log:
      allow_req_body: true
      allow_res_body: false   # prod 收紧：响应体大且可能含敏感数据（dev 可开）
  grpc:
    addr: ":8011"
    name: webook-core         # etcd 注册名，发现 key = /service/webook-core
    ttl: 30s                  # 注册租约；<=0 兜底 30s
    weight: 10                # 带权 balancer 权重
    host: ""                  # 空=自动探测出口 IP；k8s 填 POD_IP
    timeout: 5s               # unary 处理超时；streaming 豁免；<=0 兜底 5s

client:
  grpc:                       # 下游显式声明：每下游一个 ioc Provider + 一段 yaml；缺 target → dial 失败
    webook-comment:
      target: etcd:///service/webook-comment   # [必填] 无缺省推导
      balancer: breaker_swrr                   # [必填]
      timeout: 3s             # 单次调用超时；须 < 上游 server.grpc.timeout
    webook-interaction:
      target: etcd:///service/webook-interaction
      balancer: breaker_swrr
      timeout: 3s

    # ── 下游 gRPC 客户端完整可配项（= grpcx.ClientConfig）──
    # 仅 target/balancer 必填；其余为功能开关式缺省，不写=关闭/库默认，按需开启（见 §6）
    webook-xxx:
      target: etcd:///service/webook-xxx    # [必填] 解析目标，无缺省推导（缺 → dial 失败）
      balancer: breaker_swrr                # [必填] custom_swrr / breaker_swrr / group_swrr；空 → pick_first
      secure: false                         # [默认 false=insecure] true → TLS 传输
      ca_file: ""                           # secure=true 时验签 CA 证书路径；空=用系统根证书
      timeout: 3s                           # [opt-in >0 才启用] 单次调用超时；须 < 上游 server.grpc.timeout；非幂等写慎设
      max_recv_msg_size: 4194304            # [默认 0=库默认 4MB] 收消息上限（字节）
      max_send_msg_size: 0                  # [默认 0=库默认不限] 发消息上限（字节）
      health_check: false                   # [默认 false] true=开客户端健康检查（要求服务端已注册 grpc health service）
      keep_alive:                           # [opt-in time>0 才启用] 长连接保活；整段 time<=0 关闭
        time: 30s                           #   keepalive ping 间隔
        timeout: 10s                        #   ping 应答等待超时
        permit_without_stream: false        #   无活跃 RPC 流时也发 ping
      retry:                                # [opt-in 段存在即启用] 启用须过幂等/熔断/观测评审
        max_attempts: 3                     #   含首次的总尝试次数 ∈ [2,5]（grpc-go 硬上限 5）
        initial_backoff: 100ms              #   首次重试退避
        max_backoff: 1s                     #   退避上限
        backoff_multiplier: 2               #   退避倍增系数
        retryable_codes: [UNAVAILABLE]      #   触发重试的 gRPC 状态码名（大写）
        methods: []                         #   限定方法 pkg.Service 或 pkg.Service/Method；空=作用所有方法

data:
  mysql:
    dsn: root:${MYSQL_PASS}@tcp(webook-mysql:3306)/webook?parseTime=true&loc=UTC
  redis:
    addr: webook-redis:6379
    password: ${REDIS_PASS}
  kafka:
    addrs:
      - webook-kafka:9092
    dial_timeout: 10s
    read_timeout: 10s
    write_timeout: 10s
    producer_timeout: 10s     # broker ack 超时
    producer_retry_max: 3     # 0 = 失败不重试，直接降级同步发送
    metadata_retry_max: 3
    metadata_retry_backoff: 250ms
    metadata_timeout: 5s
  es:
    addr: http://webook-es:9200

etcd:
  endpoints:
    - http://webook-etcd:2379
  path: /webook-core          # 远程配置 key；不配 = 不接热更，etcd 仅供服务发现
  type: yaml

logger:
  level: info                 # debug / info / warn / error
  development: false
  encoding: json              # json / console
  output_paths: [ stdout ]
  error_output_paths: [ stderr ]

otel:
  endpoint: otel-collector:4317
  service_name: webook-core
  service_version: 0.1.0
  env: prod
  sample_ratio: 0.1           # 按环境不同：dev 1.0 / staging 0.5 / prod 0.1；改后需重启

# 业务段（llm / embedding / ollama）叶子键同样 snake_case——pkg/llm、pkg/embedding 的 config struct
# 已补 `mapstructure:"snake_case"` tag（与 infra 段一致）；timeout 是 int 秒（pkg 结构体用 int，非 duration）
llm:
  providers:
    - name: deepseek
      api_key: ${LLM_DEEPSEEK_API_KEY}
      base_url: https://api.deepseek.com
      model: deepseek-chat
      max_tokens: 2048
      timeout: 60               # 秒（int）
    - name: kimi
      api_key: ${LLM_KIMI_API_KEY}
      base_url: https://api.moonshot.cn/v1
      model: kimi-k2.5
      max_tokens: 2048
      timeout: 60

embedding:
  base_url: https://qianfan.baidubce.com/v2
  api_key: ${EMBEDDING_API_KEY}
  model: qwen3-embedding-0.6b
  dims: 1024                  # 向量维度，与 ES mapping 对齐；改需重建索引
  timeout: 30

ollama:
  base_url: http://webook-ollama:11434
  model: bge-m3
  timeout: 5

migrator:
  sdk:
    enabled: true             # false = NoOp 零开销；true = 启用切流读 + 双写
    task_name: published_article_v1
```

### 调参键默认值字典

**代码兜底默认**（yaml 应显式写出这些键；此表是 yaml 缺失 / 写 0 时的 fallback。单一登记处，实现分散在各消费方，禁止代码里另起集中默认表）：

| 键 | 默认 | 消费方 |
|----|------|--------|
| `server.http.timeout` | `15s`（SSE 路径豁免） | http 超时中间件 |
| `server.grpc.timeout` | `5s`（streaming 豁免） | grpcx unary 拦截器 |
| `server.grpc.ttl` | `30s` | `grpcx`（`defaultLeaseTTL`） |
| `server.grpc.weight` | `1` | `grpcx` / etcd resolver（`<=0` 按 1 计） |
| `server.grpc.host` | 自动探测出口 IP | `grpcx`（`netx.ExternalIp()`） |
| `client.grpc.<svc>.timeout` | `3s`（须 < 上游 server 超时） | grpcx（`methodConfig.timeout`） |
| `client.grpc.<svc>.keep_alive` | 关闭 | grpcx（段缺省关闭） |
| `client.grpc.<svc>.retry` | `nil`（不重试） | grpcx（缺省 nil） |
| `client.grpc.<svc>.max_*_msg_size` | grpc-go 库默认（recv 4MB / send 不限） | grpcx |
| `client.grpc.<svc>.secure` / `ca_file` | `false` / 空（系统根证书） | grpcx（`secure=true` 才走 TLS + CA 验签） |
| `client.grpc.<svc>.health_check` | `false` | grpcx（需服务端已注册 grpc health service） |
| `data.kafka.dial/read/write/producer_timeout` | `10s` | kafka 初始化（消费方 `if <=0` 兜底） |
| `data.kafka.producer_retry_max` / `metadata_retry_max` | `3` | kafka 初始化 |
| `data.kafka.metadata_retry_backoff` | `250ms` | kafka 初始化 |
| `llm.providers[].timeout` | `60`（秒，int） | ai client |
| `llm.providers[].max_tokens` | `2048` | ai client |
| `redislockx` watchdog | `ttl/3` | `redislockx.applyOptions`（不进 yaml） |
| `redislockx` retryInterval | `100ms` | `redislockx.applyOptions`（不进 yaml） |

### 域职责

| 域 | 职责 | 包含 |
|----|------|------|
| `server` | 本服务监听面 | `http`（addr / timeout / access_log / jwt）、`grpc`（addr / timeout / 注册元数据） |
| `client` | 下游依赖面 | `grpc.<服务名>`（target / balancer / timeout …） |
| `data` | 存储与中间件 | `mysql` / `redis` / `kafka` / `es` |
| `etcd` | 控制面：远程配置 + 服务注册发现（单实例双职责） | |
| `logger` / `otel` | 可观测 | zap 配置 / trace 导出与采样 |
| 业务段 | 服务私有配置，顶层平铺 | `llm` / `embedding` / `ollama` / `sensitive` / `ratelimit` / `migrator` |

### 键值规范

- 键名：**全叶子键 snake_case**（infra 段与业务段一律，无例外）；时间：duration 字符串，Go 侧 `time.Duration`
- **业务段绑定方式**：`llm` / `embedding` / `ollama` / `migrator` 等业务段的 config struct（`pkg/llm.ProviderConfig`、`embedding.Config` / `OllamaConfig`）已补 `mapstructure:"snake_case"` tag，与 infra 段（kafka/grpcx/logger/otel）同款；migrator pipeline 段是 `viper.GetInt("migrator.full.batch_size")` 直接取键，也走 snake_case。**注意**：viper 把 key 全小写后与字段做大小写不敏感匹配，snake_case（带下划线）**必须有 tag** 才绑得上，无 tag 只能绑 camelCase——故业务段补 tag 是 snake_case 化的前提
- **例外只在值类型**：`llm` / `embedding` / `ollama` 的 `timeout` 是 **int 秒**（pkg 结构体用 `int`，非 duration 字符串）；`ratelimit.comment.interval` 是 duration（`1m`）。键名本身无例外
- 地址：`addr` 统一 `"[host]:port"`；URL 类字段用 `base_url` / `endpoint`（带协议）
- 开关：`enabled` / `disabled` 布尔，不用 0/1
- 服务名：全仓统一 `webook-<svc>`，出现在 `server.grpc.name`、`client.grpc.<name>`、`otel.service_name`、etcd `path` 四处，必须一字不差（无全局常量，靠约定 + review）
- 密钥：`${ENV_VAR}` 占位，加载时文本展开（见 §9）

## 6. Go 侧结构（类型各归其位，无中央装配）

```
pkg/grpcx/                # gRPC 段类型由 grpcx 自带：ServerConfig / ClientConfig（+ 就地默认，无 Validate）
<svc>/config/             # 只放 yaml：local / dev / staging / prod / test（无 .go 文件）
<svc>/ioc/*.go            # 每个 Provider viper.UnmarshalKey 读自己那段 → 直接用；叶子段就地内联（如 otel.go）
```

```go
// pkg/grpcx —— gRPC server 段类型自带（[默认] 消费点就地兜底；无 Validate）
type ServerConfig struct {
    Addr    string        `mapstructure:"addr"`    // [必填] 空 → Serve / Register 报错
    Name    string        `mapstructure:"name"`    // [必填] 空 → Register 报错
    Timeout time.Duration `mapstructure:"timeout"` // [默认 5s] unary 超时；<=0 兜底；streaming 豁免
    TTL     time.Duration `mapstructure:"ttl"`     // [默认 30s] Register 内 if<=0 兜底
    Weight  int           `mapstructure:"weight"`  // [默认 1] <=0 按 1 计
    Host    string        `mapstructure:"host"`    // [默认 自动探测] 空 → netx.ExternalIp()
}

// <svc>/ioc —— 每个 Provider 读自己那段直接用；无 Bootstrap、无 Validate。
// 沿用现有 otel.go 模式：UnmarshalKey 读段 → 构造。解码错误仍返回（不 _=err），但不做值校验。
func InitGRPCServer(cli *etcdv3.Client, l logger.LoggerX) (*grpcx.Server, error) {
    var cfg grpcx.ServerConfig // grpcx 自带类型
    if err := viper.UnmarshalKey("server.grpc", &cfg); err != nil {
        return nil, err
    }
    // ttl / weight / host 缺省由 grpcx 就地兜底；addr / name 缺失 → Serve / Register 自然报错
    return grpcx.NewServer(cfg, cli, l, opts...), nil
}
```

- **无 config 包、无 Bootstrap**：`<svc>/config/` 只放 yaml；grpcx 拥有 `ServerConfig` / `ClientConfig`（+ 就地默认）；`ServerHTTP` / `Data`（mysql/redis/kafka/es）/ `Etcd` / `Logger` / `Otel` / 业务段等无专属 pkg 的叶子段在**消费它的 ioc 内联定义**（如 `otel.go` 里的 `type Config struct`）
- 每个 ioc Provider `viper.UnmarshalKey("<段>", &cfg)` 读自己那段直接用；**保留** `viper.UnmarshalKey`（不走中央 Unmarshal），解码错误按仓规返回、不 `_ = err`
- **不做值校验**（彻底不校验）：无必填 / 枚举 / 范围检查；缺失身份键 → 消费点自然失败（下见），调参键缺省 → 就地兜底（§4）

### gRPC client 显式声明（替代隐式推导）

老 `grpcClientConfig` 在 target 缺失时按 `etcd:///service/<name>` 推导，core 的 5 份 yaml 甚至完全没有 client 段、全靠推导在跑——段名拼错静默走默认值，排查成本高。改为**每个下游一个 ioc Provider + 一段 yaml，代码不派生**：

```go
// pkg/grpcx —— gRPC client 段类型由 grpcx 自带（服务名不设全局常量，取舍见本节末）
type ClientConfig struct {
    Target         string        `mapstructure:"target"`            // [必填] 无缺省推导
    Balancer       string        `mapstructure:"balancer"`          // [必填]
    Timeout        time.Duration `mapstructure:"timeout"`           // [默认 3s] 经 methodConfig.timeout；须 < 上游 handler 超时
    Secure         bool          `mapstructure:"secure"`            // [默认 false=insecure]
    CAFile         string        `mapstructure:"ca_file"`           // secure=true 时必填
    KeepAlive      KeepAlive     `mapstructure:"keep_alive"`        // [默认 关闭]
    MaxRecvMsgSize int           `mapstructure:"max_recv_msg_size"` // [默认 库默认 4MB]
    MaxSendMsgSize int           `mapstructure:"max_send_msg_size"` // [默认 库默认不限]
    HealthCheck    bool          `mapstructure:"health_check"`      // [默认 false]
    Retry          *GRPCRetry    `mapstructure:"retry"`             // [默认 nil=不重试]
}

type KeepAlive struct {
    Time                time.Duration `mapstructure:"time"`    // ping 间隔；0 = 关闭整段
    Timeout             time.Duration `mapstructure:"timeout"` // ping 应答超时
    PermitWithoutStream bool          `mapstructure:"permit_without_stream"`
}

type GRPCRetry struct {
    MaxAttempts       int           `mapstructure:"max_attempts"`       // 总尝试次数 ∈ [2,5]（grpc-go 硬上限 5）
    InitialBackoff    time.Duration `mapstructure:"initial_backoff"`
    MaxBackoff        time.Duration `mapstructure:"max_backoff"`        // ≥ initial_backoff
    BackoffMultiplier float64       `mapstructure:"backoff_multiplier"` // ≥ 1
    RetryableCodes    []string      `mapstructure:"retryable_codes"`    // gRPC 码名，交 grpc-go 组装处理；慎加 UNKNOWN / DEADLINE_EXCEEDED
    Methods           []string      `mapstructure:"methods"`            // pkg.Service 或 pkg.Service/Method；空 = 全部方法
}

// <svc>/ioc —— 每个下游一个 Provider，段名写死、显式；代码不派生 target
func InitCommentConn(cli *etcdv3.Client) (commentv1.CommentServiceClient, func(), error) {
    var cfg grpcx.ClientConfig
    if err := viper.UnmarshalKey("client.grpc.webook-comment", &cfg); err != nil {
        return nil, nil, err
    }
    conn, cleanup, err := grpcx.NewClient(cli, cfg, opts...) // 空 target → dial / resolve 失败
    if err != nil {
        return nil, nil, err
    }
    return commentv1.NewCommentServiceClient(conn), cleanup, nil
}
```

> **显式性靠 wire + yaml 结构，不靠校验**：忘配段 → cfg 零值 → target 空 → dial 失败；段名拼错（yaml 写 `webook-commnet`）→ Provider 读的 `webook-comment` 段缺失 → 同样 dial 失败。**代码始终不派生 target**，拼错不会被"能跑"掩盖（这正是替代老 `grpcClientConfig` 推导的核心）。服务名不设全局常量、无 `requiredGRPCClients` 清单、无双向校验（彻底不校验）。

### gRPC client 配置项裁决（按 grpc-go 能力 + 本项目需求）

grpc-go 拨号能力众多，逐项裁决进不进配置面：

| grpc-go 能力 | 本项目处理 | 理由 |
|-------------|-----------|------|
| target / TLS | ✅ `target`（必填）/ `secure` + `ca_file` | 身份与传输安全，显式必填 |
| 负载均衡策略 | ✅ `balancer` 必填 | 本项目自有 breaker_swrr，显式声明不吃隐式默认 |
| keepalive / 消息尺寸 | ✅ `keep_alive` / `max_recv_msg_size` / `max_send_msg_size`；缺省关闭 / 库默认，零行为变化 | 长连接保活与批量接口（BatchIncrReadCount）是真实需求；调参键，就地兜底 |
| 健康检查 | ✅ `health_check`；缺省 false | 服务端已注册 health service |
| retry（service config retryPolicy） | ✅ `retry` 段；缺省 nil 不重试 | **启用**按服务过评审（见下），yaml 不写则零行为变化 |
| 单次调用超时 | ✅ `timeout`（默认 3s）| grpcx 组 methodConfig.timeout；须 < 上游 handler 超时 |
| middleware / unary interceptor / node filter / discovery / dial option | ❌ 不进配置 | 行为注入经 `opts ...grpc.DialOption` 正交传入（otel / metrics / errconv） |
| 发现子集（subset） | ❌ 不采纳 | 面向几十上百实例的规模优化，6 服务单副本 YAGNI |

### service config 归位（loadBalancingConfig + methodConfig）

grpc-go 原生 channel 配置是一份 service config JSON。本架构**不设裸 JSON 透传键**——内容级错误（methodConfig 里 service 名拼错）grpc 不报错、只是静默不匹配，违反类型约束（§3-1）与显式原则；由 grpcx 从**类型化配置**组装：

| service config 字段 | 来源配置键 | 缺省行为 |
|--------------------|-----------|---------|
| `loadBalancingConfig` | `balancer` | 现状已实现，改结构体 marshal |
| `methodConfig.retryPolicy` | `retry` 段 | 缺省 nil 不重试 |
| `methodConfig.timeout` | `timeout` 键 | 就地默认 3s（yaml 可覆盖 / 置 0 关） |
| `healthCheckConfig` | `health_check` | 缺省 false |
| `hedgingPolicy` | 不采纳（与 retryPolicy 互斥，YAGNI） | — |

`retry` 段 schema（启用时才写进某服务 yaml）：

```yaml
retry:
  max_attempts: 3                  # 总尝试次数；grpc-go 硬上限 5
  initial_backoff: 10ms
  max_backoff: 100ms
  backoff_multiplier: 2.0
  retryable_codes: [ UNAVAILABLE ] # 慎加 UNKNOWN / DEADLINE_EXCEEDED：服务端可能已执行，非幂等写重试 = 重复生效
  methods: []                      # 限定生效的 Service/Method；空 = 全部方法（含写方法，须先确认幂等）
```

**机制（struct + grpcx 组装 service config）本次一起落，缺省全关；某服务 yaml 启用 retry 前必须过三点评审**：①重试与 breaker_swrr 交互（重试流量打到半开节点、熔断失败计数双算）；②观测口径（client 拦截器按 RPC 记一次，attempts 发生在信道内部，服务端 QPS 放大在 client 指标不可见）；③目标方法幂等性（`methods` 作用域收窄到幂等方法）。原生逃生口保持在代码面（`opts ...grpc.DialOption`），不开配置面。

### 无校验，靠自然失败 + 显式结构

不设任何配置校验（彻底不校验）。错误如何暴露：

- 缺 `target` / `balancer` → `grpcx.NewClient` 拿空 target → dial / resolve 失败（etcd 查无端点）
- 缺 `server.grpc.addr` / `name` → `Serve` / `Register` 直接返错（`grpcx.Register` 已有 `name=="" || port<=0` 的 `fmt.Errorf`）
- `secure: true` 但 `ca_file` 缺失 / 不可读 → `credentials()` 的 `os.ReadFile` 返错（`client.go` 已处理）
- `keep_alive` / `retry` / `msg size` 非法 → grpc-go 组装 service config 时按其规则处理（如 `keep_alive.time < 10s` 客户端下限）
- 段名 / 键名拼错 → 该段读空 → 落到上面某条自然失败；**代码不派生、不掩盖**

代价与缓解见 §12：迁移后每个环境都要**实际启动并跑通关键路径**（gRPC 拨号 / 连库 / 发一条 kafka），不能只 `go build`。

### 超时治理（server + client，本次一起落）

超时是行为变化（切断原先无界的调用），本次一并落地、不再拆期；长连接 / 流式**豁免**：

| 层 | 机制 | 默认（就地兜底） | 豁免 |
|----|------|----------------|------|
| HTTP server | gin 中间件设请求 ctx 超时 | 15s | SSE / 流式路径按清单豁免（镜像 `IgnoredPaths`）——chat 的 `/chat/*` |
| gRPC server | unary 拦截器包 ctx 超时 | 5s | streaming RPC 天然不被 unary 拦截器覆盖（chat 流式） |
| gRPC client | grpcx 组 `methodConfig.timeout` | 3s | 无（须 < 上游 `server.grpc.timeout`） |
| 出站 LLM / embedding | 各自 `providers[].timeout` / `embedding.timeout` | 60s / 30s | 与 server 超时正交，独立设 |

- 均为**调参键、就地兜底**：yaml 显式写出，要调直接改该值；`<=0` 用兜底默认（不支持置 0 关闭；无界流走 streaming / SSE 豁免）。
- chat 的 HTTP 超时豁免路径写进其中间件配置（SSE 不能被 15s 切断）；worker 无业务 server，超时中间件不涉及其 cron 任务。
- 链路约束：`client.timeout < 上游 server.grpc.timeout`，避免上游还在跑、下游已超时误重试。

## 7. 注释规范

只允许三类注释：

1. **取值语义**：枚举、范围、特殊值含义（`# debug / info / warn / error`、`# 0 = 不限制`）
2. **行为影响**：改动后果、生效方式（`# 改后需重启`、`# 须小于上游 handler 超时`）
3. **结构说明**：文件头 1 行 + 服务特有段的一句话（`# 远程配置 key；不配 = 不接热更`）；省略的调参键可一行注明默认（`# ttl / weight 省略 → 30s / 1`）

禁止四类：

1. 操作教程 / 命令行（`etcdctl put …`）→ 挪 runbook 或 docs
2. 公式推导、多行论述 → 挪 docs，注释只留一行文档引用
3. 跨文件重复的通用规范（环境命名尾注 6 份拷贝）→ 收敛到本文档，yaml 文件头一行引用
4. 复述键名的空话（`addr: … # Redis 地址`）

形式：文件头注释 1 行（服务特有说明至多再加 2 行）；段级注释放段上方 1 行；键级注释行尾，不超 1 行；中文短句。

## 8. 服务差异矩阵

| 段 | core | interaction | comment | chat | worker | migrator |
|----|------|-------------|---------|------|--------|----------|
| `server.http.addr` | :8010 | :8040 ¹ | :8030 | :8020 | :8050 | :8200 |
| `server.grpc` | :8011 | :8041 | :8031 | — | — | — |
| `client.grpc` | comment, interaction | — | — | core, interaction | core, interaction | — |
| `data.mysql` | webook | webook | webook | webook | — | webook_migrator |
| `data.redis` | ✓ | ✓ | ✓ | ✓ | ✓（仅 cron 锁） | ✓ |
| `data.kafka` | producer | — | — | — | consumer ² | — |
| `data.es` | ✓ | — | — | — | — | —（sink 走 `migrator.es`） |
| `etcd` 热更 path | ✓ | ✓ | ✓ | ✓ | — ³ | ✓ |
| `logger` / `otel` | 全部服务同构 | | | | | |
| 业务段 | llm, embedding, ollama, migrator.sdk | — | sensitive, ratelimit | llm | — | migrator.\*, server.http.jwt |

¹ interaction 的 HTTP 只暴露 `/metrics` + `/health`，业务全走 gRPC。
² worker 消费者键：`consumer_group`（[必填]）/ `consumer_backoff_initial`（[默认 5s]）/ `consumer_backoff_max`（[默认 60s]）。
³ worker 纯静态配置（只 `LoadLocal`），etcd 仅供服务发现，无 `path` / `type`。

## 9. 密钥管理

两级策略：

| 类别 | 处理 | 例 |
|------|------|-----|
| 外部真实凭据 | **全环境** `${ENV}` 占位，git 里绝不出现 | `LLM_DEEPSEEK_API_KEY` / `LLM_KIMI_API_KEY` / `EMBEDDING_API_KEY` |
| 自建中间件密码 | local / test 可明文（本机 docker，威胁面为零）；dev / staging / prod 占位 | `MYSQL_PASS` / `REDIS_PASS` |

- 注入机制（**解析后展开，结构上免疫 yaml 注入**）：`viperx.LoadLocal` 先读 cwd 的 `.env`（若有）注入进程环境（见下）→ `yaml.Unmarshal` 解析出配置树（模板里的 `${NAME}` 是合法 yaml 标量）→ `expandTree` 递归遍历树、**只对已解析的字符串叶子**做 `${NAME}` 展开（map / slice 全走到，覆盖 `providers[]` 等列表元素）→ `viper.MergeConfigMap` 塞回。**关键**：展开发生在「已是 Go 字符串的值」上，注入值不再经过 yaml 解析器 → 密钥含 `#` / `: ` / 换行 / 引号 / `{[` 只是普通字符，**无法破坏结构或注入伪键**（旧版在解析前对字节做替换有此隐患，已废弃）
- 展开规则：**仅 `${NAME}` 形式**（裸 `$FOO` / 孤立 `$` 原样保留，防误伤含 `$` 的值）；未设置的变量展开为空 → 消费点自然失败（如空 dsn 连库失败）；单遍替换，注入值里再含 `${...}` 不二次展开
- **残留约束（正交，非展开层能解）**：DSN 内嵌密码 `root:${MYSQL_PASS}@tcp` 若密码含 `@`/`:`/`/`/`?` 会坏 **DSN 格式**（不是 yaml 问题）——密码需 url-safe 或 url-encode；这是 DSN 语法约束，与 `${}` 展开无关。彻底规避见 L2（`BindEnv` 整条 dsn 从 env，见下）
- 变量来源：容器部署走 `deploy/.env.<env>`（docker-compose 转发进容器）；本地 `go run` / `go test` 走 **`.env` 文件**（`viperx.loadDotEnv` 用 godotenv 解析注入）或 shell / IDE 环境变量
- **单个 `.env`（放配置文件同目录）**：`LoadLocal` 读 `filepath.Dir(APP_ENV)/.env`（`config/local.yaml` → `config/.env`）——与 yaml 同目录、不依赖 CWD（yaml 能加载 = `.env` 就在旁边）。godotenv 不覆盖已存在键 → 优先级 **真实环境变量（含 IDE Run Config）> `.env`**；文件不存在直接跳过。**不做 per-env `.env.<env>` 选择**（KISS）：本地基本只跑 local.yaml,dev/staging/prod 跑在 docker 走 `deploy/.env.<env>` 转发,本地按环境分密钥的需求不存在——真需要再加（YAGNI）
- **secret 安全**：`.env` 持真实密钥必须 gitignore（绝不入仓）,只 track `.env.example` 模板。deploy 那套 `deploy/.env.<env>` 是 docker 用途、名字带后缀,与本地 `.env` 互不影响
- 复用既有 `MYSQL_PASS` / `REDIS_PASS` 变量名，**消灭「.env 与 yaml 密码手工同步」规则**（单一来源）
- 展开只作用于本地 yaml；etcd 远程内容不展开 → **密钥禁止进 etcd**
- L2 演进：变量改由 K8s Secret `envFrom` 提供，机制不变。更彻底的方案是 viper 官方 `BindEnv`（按 key 用整个 env 值覆盖，密钥整条进 env、根本不进 yaml → 连 DSN 格式约束都规避），代价是逐键 boilerplate + `.env` 改成整值——留待 L2 与 K8s Secret 一起做，L1 不引入

## 10. 加载机制与可观测

```
启动: viperx.LoadLocal(--env / APP_ENV / 缺省 config/local.yaml)
      读文件 → 展开 ${ENV}（仅 ${NAME} 形式，见 §9）→ viper.ReadConfig
      → 各 ioc Provider viper.UnmarshalKey("<段>", &cfg) 读自己那段直接用
        （解码错误返回；调参键缺省就地兜底 §4；不做值校验，配置错误落消费点）
热更: viperx.WatchRemote(etcd 段)，5s 轮询
      reload 成功 → 远程子集逐键 viper.Set(override 层，高于 file，绕开 file>kvstore)
               → 触发 ConfigChangeCallbacks → 相关 ioc 重新 UnmarshalKey 该段
      reload 失败 → 保留旧配置继续运行
指标: webook_config_reload_total{status="success|error"}（viperx 内计数）
      Grafana 告警：error 增长 或 success 长期无增量（watch 已断）
例外: worker 只 LoadLocal，配置变更靠重启
远程配置定位: etcd 只放可热更的运行参数（sample_ratio / access_log / ratelimit / migrator.sdk 等），
      不放连接串、密钥、监听地址（改这些本来就要重启）
```

## 11. 迁移映射（旧 → 新）

| 旧 | 新 | 本次 | 备注 |
|------|------|------|------|
| **全部叶子键** camelCase | snake_case（mapstructure tag 同步） | ✓ | infra 段 `dialTimeout` → `dial_timeout` 等；业务段 `apiKey`/`baseUrl`/`maxTokens`/`taskName`/`batchSize`… 一并 snake，`pkg/llm`、`embedding`、migrator pipeline 补 tag / 改取键 |
| `http.addr` | `server.http.addr` | ✓ | |
| `web.logger.*` | `server.http.access_log.*` | ✓ | 改名：它是访问日志中间件配置 |
| `web.jwt.disabled`（migrator） | `server.http.jwt.disabled` | ✓ | |
| `grpc.server.port`（int） | `server.grpc.addr`（`":8011"`） | ✓ | `grpcx.ServerConfig` Port → Addr |
| `grpc.server.{name,host}` | `server.grpc.{name,host}` | ✓ | 平移（name 必填、host 调参默认自动探测） |
| `grpc.server.ttl: 30` / `weight: 10` | `server.grpc.ttl: 30s` / `weight: 10` 显式保留 | ✓ | duration 化；代码 `defaultLeaseTTL` / `<=0` 按 1 仅兜底 |
| `grpc.client.<svc>.*` | `client.grpc.<svc>.*` | ✓ | 平移 |
| `grpcClientConfig` target 缺省推导 | 删除：每下游一 Provider + 一 yaml 段显式声明，代码不派生（无双向校验） | ✓ | 显式优于隐式（原则 6） |
| core 无 `grpc.client` 段（全靠推导） | 5 份 yaml 补 `client.grpc` 两个下游段 | ✓ | comment + interaction |
| `mysql` / `redis` / `es` | `data.*` | ✓ | 必填键平移 |
| `kafka` 各超时 / 重试（`10 # 秒` / `250 # 毫秒`） | duration 化显式保留（`10s` / `3` / `250ms` …） | ✓ | 消费方加 `if <=0` 兜底 fallback |
| `llm/ollama/embedding` 的 `timeout` / `max_tokens` | snake_case + **int 秒**（`timeout: 60` / `max_tokens: 2048`，pkg 结构体用 int，不 duration 化） | ✓ | 补 mapstructure tag；消费方 `if <=0` 兜底 fallback |
| `logger.level.l: -1` | `logger.level: debug` | ✓ | `zapcore.ParseLevel` |
| yaml 明文 apiKey / 密码 | `${ENV}` 占位（§9） | ✓ | 同步 `.env.<env>` + example |
| ioc 内 `viper.UnmarshalKey` | **保留**：内联结构体换 grpcx 自带类型 + snake_case tag + duration；无 Bootstrap | ✓ | 每下游一 Provider 读 `client.grpc.<name>` 段 |
| 无超时治理 | `server.http.timeout`(15s) / `server.grpc.timeout`(5s) / `client.grpc.<svc>.timeout`(3s) | 实施 | 中间件 + unary 拦截器 + methodConfig；chat SSE / streaming 豁免（§6 超时治理） |
| `etcd.*` / `otel.*` / `sensitive` / `ratelimit` | 键位不动，仅 snake_case 化 | ✓ | `ratelimit.comment.interval: 1m` 本就是 duration 范本 |

每服务迁移 = ioc（`UnmarshalKey` 到新段类型）+ wire + 5 份 yaml（域划分 / snake_case / duration 化 / 调参键显式保留 / `${ENV}`）+ etcd 内容重整（只留热更参数子集）+ 注释按 §7 清洗，同一窗口原子完成；**无 config 包、无 Bootstrap**。

## 12. 风险

- **调参默认下沉代码，漏迁 = 悄悄回落默认值**：某环境曾靠 yaml 显式调过的调参值（如 kafka 超时调大过），迁移删键前必须 `grep` 各 yaml 的**非默认调参值**，确认要么进代码默认、要么保留在该份 yaml，别一删了之
- **git 历史已泄露真实 key**：deepseek / kimi / dashscope apiKey 已进提交历史，删除文件 ≠ 撤销泄露，迁移后必须在供应商侧**轮换作废**
- **viper 本地/远程优先级（已定方案）**：viper 层级 file > kvstore，同名键 etcd 默认不覆盖本地——故热更**不依赖**该自动优先级：`WatchRemote` reload 后对远程子集逐键 `viper.Set`（override 层高于 file，必覆盖），本地 yaml 同名键仅作启动默认。实施时用 etcd 集成测试验证一次覆盖生效
- **etcd 远程配置滞后**：yaml 换新键但 etcd 里还是旧结构 → 新键读空。每服务迁移必须同步重整 etcd 内容，重启前完成
- **`grpcx.ServerConfig` 是公共包**：Port → Addr 属破坏性变更，core / comment / interaction 三个 gRPC 服务必须同一 PR 内同步
- **Provider 签名批量变化**：wire 全量重生成，逐服务小步走、每步 build + test
- **彻底不校验的代价**：配置错误（缺 target / 坏 dsn / 段名拼错）不在启动集中报错，落到首次消费才炸（dial / 连库 / Register），症状可能离根因稍远；换来零校验样板。缓解：迁移后 5 个环境都**实际启动并跑通关键路径**（gRPC 拨号 / 连库 / 发一条 kafka），别只 `go build`（test.yaml 由集成测试兜底）
- **超时治理是行为变化（本次一起上）**：原先无界的调用会被切断——已豁免：chat SSE 路径（HTTP 中间件清单）、gRPC streaming（unary 拦截器天然不覆盖）、出站 LLM / embedding（自带 60s / 30s）。默认放宽（HTTP 15s / gRPC 5s）+ 上线前 5 环境各压一次长调用 / 流式确认不误杀

## 13. 实施任务（一次性，不分期）

顺序：先 pkg 基建 → 逐服务迁移（面最小的先走，破坏性变更同窗口同步）→ 部署 + 收尾。

1. `pkg/viperx`：`LoadLocal` 加 `${ENV}` 占位展开（仅 `${NAME}`）；`WatchRemote` reload 后对远程子集逐键 `viper.Set`（override 层，绕开 file>kvstore，见 §10）；加 `webook_config_reload_total` 计数（**不加 ErrorUnused**）
2. `pkg/grpcx`（段类型各归其位）：`ServerConfig` Port → Addr、TTL → `time.Duration`、加 `mapstructure` tag、`weight <=0` 按 1；`ClientConfig` 扩展 keep_alive / retry / timeout / max msg size（缺省全关，`timeout` 默认 3s，组 `methodConfig`）；gRPC unary 超时拦截器（默认 5s）。**不加 Validate**
3. **超时治理**（行为变化，一起上，见 §6）：HTTP server 超时中间件（默认 15s + SSE 豁免清单）；gRPC unary 拦截器（5s）；client `methodConfig.timeout`（3s）。chat 的 `/chat/*` SSE 路径进豁免清单
4. **消费方补调参默认**：kafka 初始化 / ai client / embedding client 各加 `if <=0 用默认`（就地兜底，不集中）
5. 逐服务迁移（worker → interaction → comment → chat → migrator → core）：每服务 = ioc 各 Provider 改 `viper.UnmarshalKey` 到新段类型（grpcx 类型 + 内联叶子段，每下游一 Provider 读 `client.grpc.<name>`）+ 挂超时中间件/拦截器 + `wire ./...` + 5 yaml（域划分 / snake_case / duration / 调参键显式 / `${ENV}`）+ etcd 重整 + `go build ./... && go test ./<svc>/...` + **实际启动跑通关键路径**。无 config 包、无 Bootstrap
6. `deploy/`：`.env.<env>` + example 增 `LLM_*_API_KEY` / `EMBEDDING_API_KEY`；Grafana 加 config reload 告警
7. 收尾：已泄露 apiKey 供应商侧轮换；yaml 注释按 §7 清洗；同步 `webook/CLAUDE.md`「环境说明」+ `CHANGELOG.md`；上线前 5 环境各压一次长调用 / 流式确认超时不误杀

## 14. 决策记录

| 日期 | 决策 | 结论 |
|------|------|------|
| 2026-07-03 | 文档去框架化 | 重定稿：移除全部"对标 Kratos"措辞，设计以 grpc-go 语义 + 项目需求自证；标准实质（域划分 / snake_case / duration / 显式 client）保留（用户确认） |
| 2026-07-03 | 框架式 config schema | 不采用：不引入框架式 config loader / proto 配置定义，只用 viper + 类型化 struct，标准且可控 |
| 2026-07-03 | 类型约束 | 强类型 struct，禁 `map[string]any` / `interface{}` / 裸 JSON；**不设校验层**（无 Validate / ErrorUnused / 枚举白名单），取值交消费库自然兜住 |
| 2026-07-03 | **默认值策略（二次修订）** | 配置键（身份 + 有推荐值的调参键）yaml **全量显式写出**、可逐环境改；代码默认仅作**兜底 fallback**（yaml 缺失 / 写 0 时）。功能开关段 + 纯 pkg 内部调参（watchdog）可省。（用户二次确认：不要注释省略，万一要改） |
| 2026-07-03 | 调参默认机制 | **就地兜底**（`const` + `if <=0` / functional-option），不做集中 `applyDefaults()` / `viper.SetDefault`（用户确认：不写方法过度兼容构建） |
| 2026-07-03 | 不建 `pkg/configx` | 段类型各归其位：grpcx 自带 `ServerConfig` / `ClientConfig`（+ 就地默认），其余叶子段在**消费它的 ioc** 内联定义；服务名取消全局常量（用户确认） |
| 2026-07-03 | 无中央 Bootstrap，config 目录纯 yaml | `<svc>/config` 只放 yaml；各 ioc `viper.UnmarshalKey` 读段直接用，无 Bootstrap / MustLoad（用户确认） |
| 2026-07-03 | **彻底不校验** | 不写任何配置校验（Bootstrap.Validate / 段 Validate / 必填 / 枚举 / ErrorUnused / requiredGRPCClients 双向校验）；配置错误靠消费点自然失败暴露（用户确认，**反转「fail-fast」与「依赖清单双向校验」**） |
| 2026-07-03 | `web.logger` 去向 | 改名 `server.http.access_log`（语义准确：访问日志中间件） |
| 2026-07-03 | `logger.level` | 数字 `level.l` → 字符串枚举，随迁移一起 |
| 2026-07-03 | gRPC client 缺省推导 | 删除隐式 target 推导，改显式必填（每下游一 Provider + 一 yaml 段，代码不派生）（用户提出：过度兼容难排查） |
| 2026-07-03 | gRPC client 配置面 | 按 grpc-go 能力 + 项目需求逐项裁决：采纳 keep_alive / max msg size、health_check / retry（机制缺省关）、拒绝 subset 与代码面选项（YAGNI / 正交注入） |
| 2026-07-03 | service config 建模 | 不设裸 JSON 透传键（内容级拼错静默不生效），grpcx 从类型化键组装；retry 段 schema 定稿（retryable_codes / methods 幂等作用域，交 grpc-go 组装处理）；hedging 不采纳；原生逃生口留在代码面 opts |
| 2026-07-03 | retry / health_check | 机制（struct + grpcx 组装 service config）本次一起落，缺省全关零行为变化；启用按服务过三点评审（breaker 交互 / 观测口径 / 幂等作用域） |
| 2026-07-03 | 超时治理（不分期，一起落） | HTTP 中间件(15s，SSE 豁免) + gRPC unary 拦截器(5s) + client methodConfig.timeout(3s)；streaming / 出站 LLM 豁免。均调参键就地兜底、yaml 可覆盖（用户：不留未做完，一起做） |
| 2026-07-03 | 热更优先级 | 不依赖 viper file>kvstore；reload 后远程子集逐键 `viper.Set`(override) 必覆盖本地默认（消除原「回填」loose end） |
