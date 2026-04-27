# Grafana 配置模板参考

本目录收录 `grafana/provisioning/` 下**每种文件类型**的完整注释模板。
**Grafana 不加载本目录**（只扫 `provisioning/`），所以可以放心写详尽注释。

## 文件对照

| 真实位置（被 Grafana 加载） | 参考模板 | 主要覆盖 |
|---------------------------|---------|---------|
| `provisioning/alerting/contactpoints.yml` | `examples/alerting/contactpoints-example.yml` | 邮件模板（Alertmanager template）/ 钉钉 / Webhook / Slack |
| `provisioning/alerting/policies.yml` | `examples/alerting/policies-example.yml` | 路由 / matcher / group_by / repeat_interval / mute_times |
| `provisioning/alerting/webook-core.yml` | `examples/alerting/rules-example.yml` | HTTP / 基础设施核心告警 5 条；4 种告警范式（阈值 / 比率 / 分位数 / 容量）+ 评估链路 |
| `provisioning/alerting/<domain>.yml`（现有 `webook-core.yml` / `webook-jobs.yml`） | `examples/alerting/rules-recording-example.yml` | 配合 Recording Rules 写告警的 3 种模式（直接读 record / Counter 增量 / 比率） |
| `provisioning/dashboards/webook-jobs.json` | `examples/dashboards/webook-jobs-example.json` | 后台任务专题 14 panel（每 panel 带 description） |
| `provisioning/dashboards/dashboards.yml` | `examples/dashboards/dashboards-example.yml` | Provider 配置 + 生产三件套 |
| `provisioning/dashboards/*.json` | `examples/dashboards/webook-example.json` | 8 种 Panel 骨架 + 变量 + gridPos + 链接 |
| `provisioning/datasources/prometheus.yml` | `examples/datasources/prometheus-example.yml` | Prometheus + exemplar → Trace 联动 |
| `provisioning/datasources/zipkin.yml` | `examples/datasources/zipkin-example.yml` | Zipkin + tracesToMetrics / tracesToLogs 联动 |
| — | `examples/datasources/mysql-example.yml` | 业务库查询 + 只读账号 + Grafana 变量 |
| — | `examples/datasources/loki-example.yml` | 日志 + derivedFields（日志 → Trace 跳转） |

## 使用

### 新建配置
```bash
# 从 example 起步
cp grafana/examples/datasources/loki-example.yml \
   grafana/provisioning/datasources/loki.yml
vim grafana/provisioning/datasources/loki.yml   # 删注释、填具体值
```

### 对比差异
```bash
# 看你实际配置相对模板缺了什么
diff -u grafana/examples/alerting/contactpoints-example.yml \
        grafana/provisioning/alerting/contactpoints.yml
```

### 改动生效
- alerting rules/policies: `make -f mk/grafana.mk reload`
- contactpoints / datasources: **必须 `docker compose restart grafana`**
- dashboards: 自动扫描（30s 内），或 `make reload-dashboards`

## 深度解释

模板里每个字段的**含义、可选值、陷阱**详见：
- [docs/grafana/08-alerting-template-reference.md](../../docs/grafana/08-alerting-template-reference.md) — contactpoint 邮件模板专题（字段字典 + 函数 + 踩坑）
- [docs/grafana/09-provisioning-reference.md](../../docs/grafana/09-provisioning-reference.md) — 其它所有 provisioning 文件

## 不放在 provisioning/ 下的原因

Grafana provisioning 扫目录**按扩展名加载**所有 `.yml` / `.json`，不按后缀区分 `-example`。如果把模板放 `provisioning/` 下：

- `prometheus-example.yml` 会被当真实数据源加载 → 产生一个叫 "Prometheus Example" 的废数据源
- `webook-example.json` 会在 UI 里变真实 dashboard

所以放到**同级但独立**的 `examples/` 目录。
