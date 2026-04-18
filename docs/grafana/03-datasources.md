# 数据源管理

Grafana 不存数据，所有图表背后都是**对数据源的查询**。理解数据源是理解 Grafana 的根基。

## 一、数据源分类

| 类型 | 代表 | webook 是否用 |
|------|------|---------------|
| **时序（Time Series）** | Prometheus / VictoriaMetrics / InfluxDB / Mimir | ✅ Prometheus |
| **日志** | Loki / Elasticsearch / CloudWatch Logs | 待接入 Loki |
| **追踪** | Tempo / Jaeger / Zipkin | 待接入（OTel 已起步） |
| **关系库** | MySQL / PostgreSQL / MSSQL | 可接 webook MySQL 看业务数据 |
| **NoSQL** | MongoDB（Enterprise） / Elasticsearch | 视情况 |
| **云监控** | CloudWatch / Stackdriver / Azure Monitor | 上云后用 |
| **测试** | TestData / Random Walk | 学习用 |

每种数据源在 Grafana 里都有专属插件，提供专属查询编辑器（Prometheus 是 PromQL，Loki 是 LogQL，MySQL 是 SQL）。

## 二、access 模式

| 模式 | 说明 | 推荐 |
|------|------|------|
| `proxy` | 浏览器 → Grafana 后端 → 数据源。凭证存 Grafana，浏览器看不到 | ✅ 默认选这个 |
| `direct` | 浏览器直连数据源。**v10+ 已 deprecated** | ❌ 不要用 |

`proxy` 顺带解决跨域、隐藏内网地址、走 Grafana 鉴权。

## 三、Provisioning 数据源（生产做法）

`provisioning/datasources/<name>.yml`：

```yaml
apiVersion: 1
datasources:
  - name: <显示名>
    type: <类型 ID>
    uid: <唯一 ID，dashboard 引用用>
    access: proxy
    url: <数据源地址>
    isDefault: false
    editable: false
    jsonData:        # 公开配置
      ...
    secureJsonData:  # 加密存储的敏感配置
      ...
```

**`uid` 很重要**：Dashboard JSON 里通过 `uid` 引用数据源。手动新建数据源会随机生成 uid，搬到别的环境就断；**provisioning 强烈建议固定 uid**，环境间一致。

## 四、典型配置示例

### 4.1 Prometheus（webook 现状）

```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    uid: prometheus
    type: prometheus
    access: proxy
    url: http://webook-prometheus:9090
    isDefault: true
    editable: false
    jsonData:
      httpMethod: POST                       # 长查询用 POST，URL 不会过长
      timeInterval: "15s"                    # 默认采集间隔，rate() 计算用
      queryTimeout: "60s"
      manageAlerts: false                    # 是否让 Grafana 管理 Prometheus 的告警规则
      prometheusType: Prometheus             # 或 Mimir / Cortex / Thanos
      prometheusVersion: 2.55.0
      cacheLevel: Medium
      exemplarTraceIdDestinations:           # 与 Tempo 联动：从 exemplar 跳 trace
        - name: trace_id
          datasourceUid: tempo
```

### 4.2 Loki（日志）

```yaml
apiVersion: 1
datasources:
  - name: Loki
    uid: loki
    type: loki
    access: proxy
    url: http://loki:3100
    jsonData:
      maxLines: 1000
      derivedFields:                          # 日志中提取 trace_id 跳 Tempo
        - name: TraceID
          matcherRegex: "trace_id=(\\w+)"
          url: "$${__value.raw}"
          datasourceUid: tempo
```

### 4.3 Tempo（追踪）

```yaml
apiVersion: 1
datasources:
  - name: Tempo
    uid: tempo
    type: tempo
    access: proxy
    url: http://tempo:3200
    jsonData:
      tracesToLogsV2:                         # 从 trace 跳 Loki 日志
        datasourceUid: loki
        tags: [{key: 'service.name', value: 'service'}]
      tracesToMetrics:                        # 从 trace 跳 Prometheus 指标
        datasourceUid: prometheus
      serviceMap:
        datasourceUid: prometheus             # 服务依赖图（基于 Tempo metrics gen）
```

### 4.4 MySQL（直接查业务库）

```yaml
apiVersion: 1
datasources:
  - name: WebookDB
    uid: webook-mysql
    type: mysql
    url: webook-mysql:3306
    user: grafana_ro                          # 必须只读账号
    jsonData:
      database: webook
      maxOpenConns: 10
      maxIdleConns: 2
      connMaxLifetime: 14400
    secureJsonData:
      password: ${MYSQL_RO_PASSWORD}          # 环境变量注入，不写死
```

**MySQL 数据源的坑**：
- 用**只读账号**，禁止 `DROP/UPDATE/DELETE` 权限
- Dashboard 查询里禁止 `SELECT *`，永远 `LIMIT`
- 不要在主库上跑（拖慢业务），走从库

## 五、敏感配置：secureJsonData

凭证、Token、密码必须放 `secureJsonData`（Grafana 启动时读取，加密存元数据库；UI 不显示原值）。

```yaml
secureJsonData:
  password: "raw-password"          # 明文写在 YAML 里？
```

**不要把明文写进 Git**！两种正确做法：

**方法 A：环境变量**

```yaml
secureJsonData:
  password: $MYSQL_RO_PASSWORD
```

`$VAR` 写法 Grafana 会替换成环境变量值。docker-compose 里：

```yaml
environment:
  MYSQL_RO_PASSWORD: ${MYSQL_RO_PASSWORD}     # 来自 .env / CI secret
```

**方法 B：Vault / Secret Manager**

Grafana Enterprise 支持 Vault 集成。OSS 版只能走环境变量 + 外部秘钥管理工具填充。

## 六、数据源的"健康"

UI 上每个数据源有 "Save & test" 按钮。生产监控同样要有：

- Grafana 自身指标 `grafana_datasource_request_total` 按 `datasource` 标签看错误率
- 关键数据源（Prometheus）走 Prometheus 自身的 `up` 监控，宕了告警

## 七、跨数据源关联（这才是 Grafana 真正的杀手锏）

可观测性"金三角"——**指标 → 追踪 → 日志互跳**：

```
    Prometheus 指标图 ──exemplar(trace_id)──► Tempo Trace 火焰图
                                                  │
                                                  │ tracesToLogs
                                                  ▼
                                              Loki 日志
                                                  │
                                                  │ derivedFields(trace_id)
                                                  ▼
                                          回到 Tempo Trace
```

这是引入 OTel + Tempo + Loki 的**核心收益**。Grafana 的 `exemplarTraceIdDestinations` / `tracesToLogsV2` / `derivedFields` 三个配置撑起整套联动。

详见 OTel 文档 `05-integration.md` "与 Prometheus 协作（exemplar）"。

## 八、常见问题

| 现象 | 原因 | 解决 |
|------|------|------|
| Save & test 报 502 / dial tcp ... | URL 错（用了 localhost） | 容器网络用容器名，K8s 用 service DNS |
| 查询超时 | `queryTimeout` 太短 / Prometheus 慢 | 调 `queryTimeout`、查 Prometheus 自身性能 |
| Dashboard 数据源显示红色 ! | uid 引用了不存在的 DS | 检查 dashboard JSON 的 `datasource.uid` 与 provisioning uid 是否一致 |
| 一切正常但图表空白 | 时间范围不对（默认 last 6h） | 改成 last 15m，看 Prometheus 是否真的在采 |
| MySQL 数据源能查但巨慢 | 没用从库 / SQL 没索引 | 走从库，加索引，Query 加 LIMIT |
