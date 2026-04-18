# Grafana 概念与定位

## 一、Grafana 是什么

**Grafana** 是开源的**数据可视化与告警平台**。它本身**不存数据**，而是连接各种数据源（时序库、日志库、关系库、追踪后端），把数据画成图、组装成 Dashboard、按规则告警，并提供权限/团队/分享/通知等协作能力。

```
你以为 Grafana 是：监控
实际上 Grafana 是：    可视化前端 + 告警引擎 + 协作平台
```

由 Grafana Labs 开源（AGPL v3，自托管免费），同时提供 Grafana Cloud 商业版。当前主流稳定版本 v11.x，webook 用 v11.6.0。

## 二、能干什么

| 能力 | 说明 |
|------|------|
| **多数据源可视化** | Prometheus / Loki / Tempo / Elasticsearch / MySQL / PostgreSQL / InfluxDB / CloudWatch 等几十种 |
| **Dashboard** | 把多个 Panel 组合成业务大盘，支持变量、行折叠、链接跳转 |
| **告警（Alerting）** | 统一告警引擎（v8+），跨数据源告警，支持邮件/钉钉/Slack/Webhook |
| **Annotations** | 在图表上标注事件（发布、故障、变更），便于关联分析 |
| **用户/团队/权限** | 多组织、团队、文件夹级权限、SSO/LDAP |
| **Provisioning** | 配置即代码：数据源、dashboard、告警全用 YAML/JSON 管理 |
| **Plugins** | 数据源插件、面板插件、App 插件（如 Kubernetes / GitHub） |
| **Public Dashboard** | 一键生成对外分享链接 |
| **场景拼接（Scenes）** | v11+ 引入，dashboard 编程化 API（高级特性）|

## 三、整体架构

```
┌─────────────────────────────────────┐
│           Browser (用户)             │
└──────────────────┬──────────────────┘
                   │ HTTP
        ┌──────────▼──────────┐
        │      Grafana         │
        │  ┌────────────────┐ │
        │  │  Frontend (UI) │ │
        │  └────────────────┘ │
        │  ┌────────────────┐ │
        │  │  Backend (Go)  │ │
        │  │  ┌──────────┐  │ │
        │  │  │ DS Proxy │  │ │← 转发查询，避免浏览器跨域/暴露凭证
        │  │  │ Alerting │  │ │← 评估告警规则
        │  │  │ Auth     │  │ │
        │  │  └──────────┘  │ │
        │  └────────────────┘ │
        │  ┌────────────────┐ │
        │  │  SQLite/MySQL/ │ │← Grafana 自身元数据库（用户、dashboard）
        │  │  PostgreSQL    │ │
        │  └────────────────┘ │
        └──────────┬──────────┘
                   │
         ┌─────────┼─────────┬──────────┐
         ▼         ▼         ▼          ▼
    ┌────────┐ ┌──────┐ ┌───────┐ ┌─────────┐
    │Prometheus│ │ Loki │ │ Tempo │ │ MySQL   │
    │ (指标)  │ │(日志)│ │(追踪) │ │(业务库) │
    └────────┘ └──────┘ └───────┘ └─────────┘
```

### 关键概念

| 概念 | 说明 |
|------|------|
| **Data Source（数据源）** | 配置一次，多 dashboard 复用 |
| **Dashboard** | JSON 文件，包含若干 Panel + 变量 + 设置 |
| **Panel** | 单个图表（Time series / Stat / Gauge / Table / ...）|
| **Variable** | 动态参数（如 `$instance`），实现"一套 dashboard 看多实例" |
| **Annotation** | 时间点标注，可手动加或来自数据源查询 |
| **Folder** | dashboard 分组，权限以 folder 为单位 |
| **Organization** | 多租户隔离（社区版基本一个 org 用就行） |
| **Provisioning** | 配置文件加载机制（启动时读 `/etc/grafana/provisioning/`） |

## 四、Grafana 在可观测性栈里的位置

Grafana Labs 主推的全家桶（"LGTM Stack"）：

| 层 | 工具 | 信号 |
|----|------|------|
| Metrics | **Mimir** / Prometheus / VictoriaMetrics | 指标 |
| Logs | **Loki** | 日志 |
| Traces | **Tempo** | 追踪 |
| Profiles | **Pyroscope** | 性能剖析 |
| Frontend | **Grafana** | 可视化 + 告警 |

**webook 当前**：Prometheus（已有）→ Grafana（已有），后续接 Loki + Tempo。

### 与各组件的对照

| 工具 | 角色 | webook 是否用 |
|------|------|---------------|
| Prometheus | 时序存储 + 查询 | ✅ 已部署 |
| Alertmanager | 告警路由分发 | ❌（当前用 Grafana Alerting 替代） |
| Loki | 日志存储（类 Prometheus 的设计） | 待接入 |
| Tempo | Trace 存储（接 OTLP） | 待接入 |
| Grafana | 上述所有的统一前端 | ✅ 已部署 |

## 五、Grafana Alerting vs Prometheus Alertmanager

Grafana v8+ 内置**统一告警系统（Unified Alerting）**，可以替代 Alertmanager。两者关系：

| 维度 | Prometheus + Alertmanager | Grafana Alerting |
|------|---------------------------|------------------|
| 规则定义位置 | Prometheus 配置文件 | Grafana UI / Provisioning |
| 评估引擎 | Prometheus 自身 | Grafana 后端 |
| 数据源 | 仅 Prometheus | 任意数据源（Loki/MySQL 也行） |
| 路由 / 分组 / 静默 | Alertmanager | Grafana 内置 |
| 通知 | Alertmanager | Grafana 内置 |
| 适合 | Prometheus-only 栈 | 多数据源 / 想统一管理 |

**webook 选 Grafana Alerting**：数据源多元化（未来 Loki/Tempo），告警规则也能基于业务库 MySQL，统一在 Grafana 管理省心。详见 `05-alerting.md`。

## 六、利与弊

### 优点

- **生态最大**：数据源插件几十种，开箱即用
- **可视化能力强**：图表类型齐全，自定义灵活
- **配置即代码**：Provisioning 让 Dashboard / DS / Alert 全部 Git 化
- **统一前端**：指标 + 日志 + 追踪一站式
- **协作完善**：组织、团队、权限、分享、版本历史
- **社区活跃**：模板丰富（grafana.com/dashboards 数千个 ID 直接导入）

### 缺点 / 代价

- **复杂度**：变量、模板、Repeat、Transform 学习曲线陡
- **告警侧重新设计**：v8 之前 Legacy Alerting 与 v8+ Unified Alerting 概念不同，迁移有坑
- **元数据库**：默认 SQLite，多实例 / HA 必须切 MySQL/Postgres
- **JSON Dashboard 难手写**：靠 UI 拖拽生成，code review 时 diff 不直观
- **权限粒度限制**：社区版到 folder 级；row 级权限要 Enterprise
- **AGPL 许可**：自己改 Grafana 源码并对外提供服务时要开放修改
- **资源消耗**：默认 256MB 够小用，dashboard 多/查询重时要扩

### 何时**不需要** Grafana

- 单服务、单数据源、看一眼就行 → Prometheus 自带 UI 够用
- 团队规模小且都用 CLI → 直接 `promtool query` 或 `mtail`
- 只关心日志且量小 → `tail -f` + `grep` / Kibana

webook 多数据源 + 团队协作 + 告警需求 → Grafana 是合理选择。

## 七、版本与许可证

| 版本 | 许可 | 区别 |
|------|------|------|
| Grafana OSS | AGPL v3 | 全功能开源，社区版 |
| Grafana Enterprise | 商业 | 加企业级特性（行级权限、SSO 增强、报表、白标） |
| Grafana Cloud | SaaS | 托管版，按用量付费 |

webook 用 OSS 版足够。AGPL 注意点：**自己 fork 改源码并提供 SaaS 服务**才需要开源你的修改；自用、内部部署不受影响。
