# 最佳实践与踩坑清单

## 一、Dashboard 设计

### 命名

| 对象 | 规范 | 示例 |
|------|------|------|
| Dashboard title | `<Project> / <Scope>` | `Webook / Overview` |
| Folder | 大写单词 | `Webook`、`Infra` |
| Panel title | 描述指标本质，非 PromQL | `HTTP P99 响应时间` 而非 `histogram_quantile(...)` |
| Variable name | 小写下划线 | `instance`、`env` |
| uid | `<project>-<scope>` | `webook-overview` |

### 布局

- 24 列 grid，常用宽度：`w=8`（三栏）/ `w=12`（双栏）/ `w=24`（满宽）
- 关键指标放顶部（Stat 大数字一排：QPS / 错误率 / P99 / 在线数）
- 同主题 panel 放同一行
- 复杂 dashboard 用 Row 折叠分组

### 单位 & 阈值

- 所有时间统一用 seconds（让 Grafana 自动选 ms/s/min 显示）
- 内存统一 bytes（IEC 二进制：KiB/MiB/GiB）
- 百分比用 `percent (0.0-1.0)`，查询里 `0.95` 而不是 `95`
- 错误率 / 延迟类设阈值色块（绿/黄/红），一眼看出健康度

## 二、PromQL 在 Grafana 里的细节

### `$__rate_interval` vs `[5m]`

```promql
rate(webook_http_requests_total[5m])           # 固定 5m
rate(webook_http_requests_total[$__rate_interval])  # Grafana 自动算
```

**`$__rate_interval`** 是 Grafana 4.0+ 内置变量，值 = `max(scrape_interval × 4, $__interval)`。在长时间窗口（24h）下不会因为步长变大而采样不足。**生产推荐统一用它**。

### `$__interval`

数据点之间的步长，Grafana 根据时间窗口 + 图表宽度自动算。给 `rate()` 用 `$__rate_interval`，给其他场景用 `$__interval`。

### Legend 模板

```
{{instance}} - {{pattern}}
```

不写 legend 默认显示完整 metric + label，丑且占地。

## 三、Variable 设计

### 必备变量

```
$datasource    （Datasource 类型，多 Prometheus 切换；单一 DS 可省）
$instance      （Query: label_values(up, instance)）
$env           （Custom: dev,staging,prod；或 label_values(..., env)）
$pattern       （Query: label_values(webook_http_requests_total, pattern)）
```

### 配套设置

- **Multi-value**：勾上，允许多选
- **Include All option**：勾上，提供 "All"
- **All value**：填 `.*`（PromQL 正则全匹配）；不填默认是 Grafana 自己拼，可能拼错
- **Refresh**：选 "On time range change"（不要 "On dashboard load"，否则换时间不刷）
- **Sort**：按字母排序，不要默认无序

### 多选时查询写法

```promql
sum by (pattern) (
  rate(webook_http_requests_total{instance=~"$instance"}[$__rate_interval])
)
```

`=~"$instance"` 用正则匹配，多选时 Grafana 自动渲染成 `(a|b|c)`。

## 四、性能

### 查询性能

| 现象 | 原因 | 解法 |
|------|------|------|
| 图表加载 5+ 秒 | 时间窗口太大 / 步长太小 / 序列爆炸 | 用 `$__rate_interval`、缩短窗口、减少 series（聚合） |
| Grafana 直接报 timeout | 单查询超过 `queryTimeout` | 改 query 或调 timeout |
| Prometheus CPU 高 | 多人同时打开同一 dashboard | 加 query cache / 减 panel 数 |

### Dashboard 性能

- 单 dashboard panel 数 < 30
- 每 panel query 数 < 5
- 自动刷新 ≥ 30s（5s 会打爆 Prometheus）
- Repeat panel 控制变量基数（< 10）

### 浏览器性能

- Time series panel 控制 series 数（< 100），用 `topk(10, ...)` 限制
- Table panel 控制行数（< 1000）
- Panel 总数过多时用 Row 折叠默认收起

## 五、协作

### Code review 看什么

| 项 | 检查 |
|----|------|
| `uid` | 是否手动指定且唯一 |
| `id` / `version` | 是否清掉 |
| `datasource` | 用 uid 不用 name |
| 单位 | 是否设了 unit |
| Legend | 是否有 `{{var}}` 模板 |
| 变量 | 是否 `Include All` 且 All 值合理 |
| 阈值 | 关键 panel 是否有阈值色块 |
| 时间范围 | 默认是否合理（不要 last 24h，浪费资源） |
| 命名 | title / tags 是否符合规范 |

### 文档化

每个 dashboard 顶部加 Text panel 说明：

```markdown
**Webook 总览大盘**

- 数据源：Prometheus
- 用途：On-call 第一眼看
- 配套告警：webook-http、webook-runtime
- Runbook: [wiki link]
```

## 六、告警最佳实践

参考 `05-alerting.md` 第九节，核心：

1. 可操作（带 runbook）
2. 强制 `for: 5m`，避免抖动
3. `group_by` 合并同类
4. 基于 SLO 不基于固定阈值
5. 每个告警有 owner（label 标 team）
6. 静默有期限，禁止"永久静默"

## 七、安全

| 项 | 推荐 |
|----|------|
| admin 默认密码 | 必改 |
| 匿名访问 | 关 |
| 用户注册 | 关 |
| HTTPS | 必开（反代或内置） |
| 数据源凭证 | 走环境变量，不写死 |
| 数据源 access | 用 proxy 不用 direct |
| Embedding | 关闭 `GF_SECURITY_ALLOW_EMBEDDING`（除非必要） |
| API Key | 短有效期，用 service account 替代 legacy API key |
| 审计日志 | 开 `GF_LOG_FILTERS: audit:debug` 或 Enterprise audit |

## 八、踩坑清单（按出现频率）

| # | 坑 | 表现 | 修复 |
|---|----|------|------|
| 1 | UI 改了 dashboard 没保存到 Git | 重启后丢失 / 环境间不一致 | 提 PR 把 JSON 入仓 |
| 2 | dashboard JSON 含 `id` / `version` | PR diff 噪音大 / 跨环境冲突 | 入仓前 jq 删字段 |
| 3 | datasource uid 不固定 | dashboard 跨环境断 | provisioning 固定 uid |
| 4 | 没设 Panel unit | 显示 `1234567890`（实际是字节数） | 选 `bytes (IEC)` |
| 5 | `[5m]` 在长窗口下采样不足 | 24h 窗口曲线断断续续 | 改 `$__rate_interval` |
| 6 | Variable 没勾 "Include All" | 默认必须选一个，Dashboard 加载就报错 | 勾上 + All value `.*` |
| 7 | Repeat by variable，变量值几十个 | 浏览器卡死 | 限制变量基数 / 不用 Repeat |
| 8 | 自动刷新 5s | Prometheus CPU 高 | 改 30s 起步 |
| 9 | 告警没设 `for` | 抖一下就告警，钉钉刷屏 | `for: 5m` |
| 10 | Provisioning `editable: true` | 有人 UI 偷偷改，下次重启回滚，"诡异" | 三件套全关 |
| 11 | 容器挂 volume 权限 denied | 起不来 | `chown -R 472:472` |
| 12 | datasource url 用 localhost | 容器里连不上 | 用容器名（container_name 或 service name） |
| 13 | `editable: false` 后 admin 也改不了 | "我是管理员怎么也改不了" | 设计如此，改 YAML |
| 14 | 元数据库默认 SQLite | 多实例无法同步 | 切 MySQL/Postgres |
| 15 | 升级跨大版本没看 release notes | 告警规则迁移失败 | 必读 upgrade guide |
| 16 | Dashboard 数百个挤一个 folder | 找不到 | 按业务/环境分 folder |
| 17 | MySQL 数据源用业务主库 | 复杂 SQL 拖慢业务 | 走只读账号 + 从库 |
| 18 | 没监控 Grafana 自身 | 告警系统挂了没人知道 | Prometheus scrape Grafana `/metrics` |

## 九、个人成长路径

刚开始接触 Grafana：

1. **第一周**：跑通 docker-compose，导入社区模板（Go 6671、Gin 14031），看图
2. **第二周**：自己写 PromQL，从复制别人的 panel 开始改
3. **第三周**：学 Variable，做"按实例切换"的总览大盘
4. **第四周**：写第一条告警 + 通道（钉钉），故意触发测试
5. **第五周**：把 dashboard JSON 入 Git，体验 provisioning
6. **第六周**：CI lint + CD 自动部署
7. **第七周**：接入 Loki / Tempo，玩跨数据源跳转
8. **持续**：根据故障复盘补告警、补 panel，让 dashboard "活着"

## 十、推荐资源

- 官方文档：https://grafana.com/docs/grafana/latest/
- Dashboard 市场：https://grafana.com/grafana/dashboards/
- Plugin 市场：https://grafana.com/grafana/plugins/
- Dashboard linter：https://github.com/grafana/dashboard-linter
- Grizzly（Dashboard CLI 同步）：https://github.com/grafana/grizzly
- 官方 Best Practices：https://grafana.com/docs/grafana/latest/best-practices/
- SRE 视角的告警哲学：Google SRE Book - Practical Alerting
