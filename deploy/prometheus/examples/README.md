# Prometheus 配置模板参考

本目录的 `-example.yml` 是**完整注释**的参考模板。
Prometheus 只加载 `--config.file` 指向的文件，**examples/ 不会被加载**。

## 文件对照

| 真实位置 | 参考模板 | 主要覆盖 |
|---------|---------|---------|
| `prometheus.yml` | `examples/prometheus-example.yml` | 主配置全字段：global / alerting / rule_files / scrape_configs / remote_write / relabel |
| `prometheus.local.yml` | 同上 | 本地开发版（windows host 直连） |
| （可选）`rules/*.rules.yml` | `examples/alerts-example.yml` | Alert rules + Recording rules（原生 Prometheus 格式，供 Alertmanager 用） |
| `rules/<domain>.rules.yml`（现有 `webook-jobs.rules.yml`） | `examples/recording-rules-example.yml` | Recording Rules 模板：按业务域分文件，Prometheus 自动加载 `rules/*.rules.yml` |

## 使用

### 按 example 起步改 prometheus.yml
```bash
# 备份现有
cp prometheus/prometheus.yml prometheus/prometheus.yml.bak

# 参考 example 里的字段扩展
diff -u prometheus/examples/prometheus-example.yml prometheus/prometheus.yml
```

### 新加 Exporter 目标
在 `prometheus.yml` 的 `scrape_configs` 末尾追加 job（参考 example 的 static_configs / file_sd / kubernetes_sd 多种形式）。

### 新加告警规则（原生 Prometheus 方式）
```bash
# 1. 创建 rules 目录
mkdir -p prometheus/rules

# 2. 基于 alerts-example.yml 新建
cp prometheus/examples/alerts-example.yml prometheus/rules/webook.rules.yml
vim prometheus/rules/webook.rules.yml

# 3. 在 prometheus.yml 里启用
# rule_files:
#   - "rules/*.rules.yml"

# 4. 校验语法
docker exec webook-prometheus promtool check rules /etc/prometheus/rules/webook.rules.yml

# 5. 热加载（无需重启）
curl -X POST http://192.168.150.101:9090/-/reload
# 前提：prometheus 启动要加 --web.enable-lifecycle
```

### 改动生效
- **有 `--web.enable-lifecycle`**：`curl -X POST http://host:9090/-/reload`
- **无**：`docker compose restart prometheus`
- 配置错误 Prometheus 会拒绝 reload 保留旧配置；看日志 `docker logs webook-prometheus`

## 当前项目采用哪种告警方式

**项目目前用 Grafana Alerting**（见 `grafana/provisioning/alerting/`），不用 Prometheus 原生 alerting + Alertmanager。

两种方式的对比：

| 维度 | Prometheus + Alertmanager | Grafana Alerting（当前）|
|------|---------------------------|------------------------|
| 规则定义 | YAML 文件 | Grafana UI / provisioning YAML |
| 评估引擎 | Prometheus 自己 | Grafana 后端 |
| 数据源 | **只能 Prometheus** | 任意（含 Loki / MySQL）|
| 通知路由 | Alertmanager | Grafana 内置 |
| 告警本身的可观测性 | 弱 | 强（UI / API） |

Recording rules（预计算）**即使用 Grafana Alerting 也推荐在 Prometheus 这边配**——纯 PromQL 层预计算，跨告警/dashboard 复用，开销小。

## 相关文档

- [Prometheus 官方配置文档](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)
- [告警规则语法](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
- [Recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/)
- 本项目的 Grafana 告警参考：`docs/grafana/08-alerting-template-reference.md` + `docs/grafana/09-provisioning-reference.md`
- 监控实战：`docs/prometheus/`
