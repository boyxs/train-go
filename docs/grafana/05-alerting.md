# Grafana Alerting

Grafana v8 引入 **Unified Alerting**，把告警从 dashboard 解耦，独立成一套子系统：定义规则 → 评估 → 路由 → 通知 → 静默 → 反馈。

## 一、告警子系统组成

```
Alert Rule（告警规则）
   │  评估（每 N 秒查询一次数据源，看是否满足条件）
   ▼
Alert Instance（告警实例）
   │  ┌── Normal（正常）
   │  ├── Pending（达到条件，但还在 for 等待期）
   │  ├── Firing（持续满足，触发）
   │  └── No Data / Error（查询无数据 / 出错）
   ▼
Notification Policy（路由策略）
   │  按 label 匹配 → 选择 contact point + 分组 + 静默
   ▼
Contact Point（通知通道）
   │  Email / Slack / 钉钉 Webhook / PagerDuty / OpsGenie / Webhook
   ▼
（人收到告警）
```

补充组件：
- **Silence（静默）**：临时屏蔽某些告警（值班维护时）
- **Mute Timing（定时静默）**：周期性静默（如周末非紧急告警）
- **Templates（模板）**：通知文案模板，复用 message
- **Alert State History**：告警历史，可视化回溯

## 二、Grafana Alerting vs Prometheus Alertmanager

| 维度 | Prometheus + Alertmanager | Grafana Alerting |
|------|---------------------------|------------------|
| 规则定义位置 | Prometheus 配置 + PromQL | Grafana UI / Provisioning |
| 评估引擎 | Prometheus 自身 | Grafana 后端 |
| 数据源 | 仅 Prometheus | 所有数据源（Loki / MySQL / Tempo …） |
| 多步表达式 | 单一 PromQL | A → B → C 链式（reduce / math / threshold） |
| 路由分发 | Alertmanager | Grafana 内置 |
| 静默 | Alertmanager UI / amtool | Grafana UI |
| HA | Alertmanager gossip 集群 | Grafana 后端集群（需共享 DB） |
| 学习成本 | Alertmanager YAML | Grafana UI（直观但底层一样要懂） |

**webook 选 Grafana Alerting**：
- 数据源不止 Prometheus（未来 Loki/Tempo），统一管理省心
- 业务告警（"今天注册数 < 100" 查 MySQL）也能做
- UI 调试规则更直观

也可以**混用**：基础设施告警走 Prometheus + Alertmanager，业务/可观测性告警走 Grafana Alerting。

## 三、规则结构（"多步表达式"）

Grafana 告警最强的就是**多步 query/expression**，每步产出可以喂给下一步：

```
[A] Query: rate(webook_http_requests_total{status=~"5.."}[5m])     ← Prometheus 查询
[B] Reduce: last() of A                                            ← 取最后一个值
[C] Math:   $B / $D                                                ← 算错误率
[D] Query: rate(webook_http_requests_total[5m])                    ← 总量
[E] Threshold: $C > 0.01                                           ← 错误率 > 1% 告警
```

**Reduce 函数**：last / mean / min / max / sum / count
**Math 表达式**：`$A + $B * 100`
**Threshold**：`>` / `<` / within range / outside range

## 四、规则类型

| 类型 | 评估位置 | 适用 |
|------|---------|------|
| **Grafana managed** | Grafana 后端 | 多数据源、复杂表达式、Loki 日志告警 |
| **Data source managed** | 数据源端（如 Prometheus） | Prometheus 原生告警，规则直接下发 Prometheus |

webook 推荐 **Grafana managed**（统一管理）。

## 五、配置示例（Provisioning）

`provisioning/alerting/rules.yml`：

```yaml
apiVersion: 1
groups:
  - orgId: 1
    name: webook-http
    folder: Webook
    interval: 1m                                  # 评估周期
    rules:
      - uid: webook-http-error-rate
        title: HTTP 5xx 错误率 > 1%
        condition: E                              # 哪一步是触发条件
        data:
          - refId: A
            relativeTimeRange: { from: 300, to: 0 }
            datasourceUid: prometheus
            model:
              expr: 'sum(rate(webook_http_requests_total{status=~"5.."}[5m]))'
              refId: A
          - refId: B
            relativeTimeRange: { from: 300, to: 0 }
            datasourceUid: prometheus
            model:
              expr: 'sum(rate(webook_http_requests_total[5m]))'
              refId: B
          - refId: C
            datasourceUid: __expr__
            model:
              type: math
              expression: '$A / $B'
              refId: C
          - refId: E
            datasourceUid: __expr__
            model:
              type: threshold
              expression: 'C'
              conditions:
                - evaluator: { type: gt, params: [0.01] }
              refId: E
        for: 5m                                   # 持续多久才触发
        noDataState: NoData
        execErrState: Error
        annotations:
          summary: 'webook HTTP 5xx 错误率 {{ $values.C.Value | humanizePercentage }}'
          runbook_url: 'https://wiki.internal/runbook/webook-5xx'
        labels:
          severity: critical
          team: backend
```

`provisioning/alerting/contactpoints.yml`：

```yaml
apiVersion: 1
contactPoints:
  - orgId: 1
    name: dingtalk-backend
    receivers:
      - uid: dingtalk-backend
        type: webhook
        settings:
          url: https://oapi.dingtalk.com/robot/send?access_token=$DINGTALK_TOKEN
          httpMethod: POST
        secureSettings: {}
```

`provisioning/alerting/policies.yml`：

```yaml
apiVersion: 1
policies:
  - orgId: 1
    receiver: dingtalk-backend
    group_by: ['alertname', 'severity']
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 4h
    routes:
      - matchers: ['severity = critical']
        receiver: pagerduty-oncall
        continue: false
      - matchers: ['team = data']
        receiver: dingtalk-data
```

## 六、关键参数

| 参数 | 含义 | 推荐 |
|------|------|------|
| **interval** | 多久评估一次规则 | 1m（生产基线，业务告警可 5m）|
| **for** | 满足条件持续多久才 Firing | 5m（避免抖动）|
| **noDataState** | 查不到数据时的状态 | `NoData`（独立处理）/ `OK`（视为正常） |
| **execErrState** | 评估出错时的状态 | `Error`（关注 Grafana 自身） |
| **group_wait** | 同组首次告警等待，攒一波 | 30s |
| **group_interval** | 同组后续告警间隔 | 5m |
| **repeat_interval** | 持续 firing 的重复提醒 | 4h（不要 1h，钉钉会被刷爆） |

## 七、通知通道（Contact Points）

| 通道 | 适用 |
|------|------|
| Email | 默认；非紧急 |
| Slack | 国外团队 |
| **钉钉 Webhook** | 国内团队主流（webook 推荐） |
| 企业微信 Webhook | 同上 |
| 飞书 Webhook | 同上 |
| **PagerDuty / OpsGenie** | On-call 体系（带升级、值班轮换） |
| Webhook（自定义） | 接自己系统、对接告警平台 |

### 钉钉 Webhook 模板示例

```yaml
contactPoints:
  - name: dingtalk
    receivers:
      - type: webhook
        settings:
          url: https://oapi.dingtalk.com/robot/send?access_token=xxx
          httpMethod: POST
        # message body 用 templates 渲染
```

钉钉机器人**安全设置**必须配（关键词 / IP 白名单 / 加签），否则收不到。

## 八、告警分级

行业惯例 4 级：

| Severity | 含义 | 通知方式 |
|----------|------|---------|
| **critical / P0** | 服务不可用 / 数据丢失 | 电话 + 短信 + 即时通讯 |
| **high / P1** | 核心功能受影响 | 即时通讯 + 邮件 |
| **medium / P2** | 性能下降 / 部分功能 | 即时通讯 |
| **low / P3** | 趋势警告 / 容量预警 | 邮件 / 工单 |

通过 label 标记：

```yaml
labels:
  severity: critical
```

Notification policy 按 severity 路由到不同通道。

## 九、告警的最佳实践

### 1. **可操作（Actionable）**
告警必须能让人知道"该做什么"。每个告警挂 `runbook_url`。

### 2. **避免告警风暴**
- 强制 `for: 5m`（短抖动不告）
- `group_by` 合并同类告警
- 大故障时设 silence（不要让 100 条同类告警淹没）

### 3. **告警金字塔**
- 大量低级告警（Email 工单）→ 看趋势
- 少量中级告警（即时通讯）→ 处理
- 极少数高级告警（电话）→ 立即响应

### 4. **基于 SLO 而非阈值**
不要随便设 "QPS > 1000 告警"，应该是 "错误率超过 SLO" / "P99 超过承诺"。

### 5. **每个告警有 owner**
label 加 `team`，分发到对应团队，避免无人认领。

### 6. **静默纪律**
临时维护必加 silence（带 comment + 到期时间），杜绝 "永久静默"。

## 十、生产告警清单（webook 起步版）

| 告警 | PromQL（示意） | severity |
|------|---------------|----------|
| 服务不可达 | `up{job="webook"} == 0` for 1m | critical |
| 5xx 错误率 > 1% | `sum(rate(...status=~"5.."[5m])) / sum(rate(...[5m])) > 0.01` for 5m | high |
| P99 > 500ms | `histogram_quantile(0.99, ...) > 0.5` for 10m | medium |
| Goroutine 暴涨 | `go_goroutines > 10000` for 5m | high |
| 堆内存 > 80% | `go_memstats_heap_inuse_bytes / heap_sys > 0.8` for 5m | medium |
| MySQL 连接接近上限 | `mysql_global_status_threads_connected / mysql_global_variables_max_connections > 0.8` | high |
| Redis 内存 > 80% | `redis_memory_used_bytes / redis_memory_max_bytes > 0.8` | high |
| Kafka 消费 lag 高 | `kafka_consumergroup_lag > 10000` for 5m | medium |

详细规则见 `docs/prometheus/05-alerting.md`，本章只讲 Grafana 侧的承载方式。

## 十一、调试技巧

| 问题 | 排查 |
|------|------|
| 规则不触发 | 看 "Alert Rule" 详情页的 "Query and condition" 实时评估结果 |
| 通知收不到 | "Contact points" → "Test"，单独测通道；看 Grafana 日志 alerting 模块 |
| 告警频繁抖动 | 加大 `for` / `group_interval` |
| Provisioning 后 UI 看不到 | 检查 `apiVersion: 1`、folder 是否存在、查看 `provisioning` 目录的容器日志 |
| Notification policy 走错路 | "Notification policies" 里的 "Show matching contact points"，输入 label 模拟 |
