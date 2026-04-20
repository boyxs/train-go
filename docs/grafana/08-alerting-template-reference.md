# Grafana Alerting 模板完整参考

写 / 改 `deploy/grafana/provisioning/alerting/contactpoints.yml` 的 `subject` / `message` 时的**所有可用字段 + 函数 + 陷阱**。

## 一、模板引擎说明

- Grafana Alerting 底层用 **Alertmanager template**（不是完整 Go text/template）
- 语法基础：Go template 语法，**但上下文类型是 Alertmanager 定义的**
- 文档：<https://prometheus.io/docs/alerting/latest/notifications/#data-structures>

## 二、Top-level 上下文字段

模板根（`{{ . }}`）直接可用：

| 字段 | 类型 | 示例值 | 说明 |
|------|------|--------|------|
| `.Receiver` | string | `webook-email` | 当前 Contact Point name |
| `.Status` | string | `firing` / `resolved` | 整组告警的整体状态 |
| `.Alerts` | Alerts | `[Alert...]` | **这一批次**的所有告警（组内，不包括其它分组）|
| `.Alerts.Firing` | []Alert | 同上筛选 | 只包含 firing 状态的 |
| `.Alerts.Resolved` | []Alert | 同上筛选 | 只包含 resolved 状态的 |
| `.GroupLabels` | KV | `{alertname: X, severity: Y}` | 本组共同分组标签（policies.yml 的 `group_by`）|
| `.CommonLabels` | KV | `{team: backend, ...}` | 组内所有告警的共同 label 交集 |
| `.CommonAnnotations` | KV | `{runbook_url: ...}` | 组内所有告警的共同 annotation 交集 |
| `.ExternalURL` | string | `http://grafana/` | Grafana 自身 URL（供跳转用） |

**关键点**：`KV` 类型**不能用 `range $k, $v := map` 解包**，必须用 `.SortedPairs`（见下）。

## 三、每条 Alert 的字段（必须在 `{{ range .Alerts }}` 里）

| 字段 | 类型 | 示例 | 说明 |
|------|------|------|------|
| `.Status` | string | `firing` / `resolved` | 单条告警状态 |
| `.Labels` | KV | `{alertname: X, severity: Y}` | 该条告警的完整 labels |
| `.Annotations` | KV | `{summary: ..., runbook_url: ...}` | annotations |
| `.StartsAt` | time.Time | `2026-04-19T05:00:00Z` | 告警开始时间 |
| `.EndsAt` | time.Time | `0001-01-01T00:00:00Z`（未结束）/ 真实时间（已 resolved） | 告警结束时间 |
| `.GeneratorURL` | string | Grafana 规则详情 URL | 跳转查看规则 |
| `.SilenceURL` | string | Grafana silence 新建 URL | 一键静默 |
| `.DashboardURL` | string | （如果规则关联 dashboard）| Grafana dashboard |
| `.PanelURL` | string | 同上 | 具体 panel |
| `.Fingerprint` | string | hex | 告警唯一指纹（去重用） |
| `.Values` | map[string]float64 | `{A: 0.5, B: 100}` | **Grafana 特有**：规则表达式各 refId 的数值结果 |
| `.ValueString` | string | `[ var='A' value=0.5 ]...` | 预格式化的 values 字符串 |

**Grafana 扩展字段**（Alertmanager 标准没有的）：
- `.Values` / `.ValueString` — 可以拿告警表达式的实际数值
- `.ImageURL` — 如果有图表截图

## 四、KV 类型的使用（重点）

`.Labels` / `.Annotations` / `.GroupLabels` / `.CommonLabels` / `.CommonAnnotations` 都是 KV。

### 4.1 直接按 key 取值

```
{{ .Labels.alertname }}
{{ .Labels.severity }}
{{ .Annotations.summary }}
```

### 4.2 迭代（必须用 SortedPairs，不能 range $k, $v）

❌ **错**（Alertmanager template 不支持）：
```
{{ range $k, $v := .Labels }}{{ $k }}={{ $v }}{{ end }}
```

✅ **对**：
```
{{ range .Labels.SortedPairs }}{{ .Name }}={{ .Value }} {{ end }}
```

### 4.3 其它方法

| 方法 | 说明 |
|------|------|
| `.SortedPairs` | 返回 `[]Pair`，每个 `.Name` `.Value`，按 key 排序 |
| `.Names` | 返回 `[]string`，所有 key |
| `.Values` | 返回 `[]string`，所有 value |
| `.Remove (keys)` | 去掉指定 keys 后的新 KV |

## 五、Alerts 类型的扩展方法

```
{{ len .Alerts }}           # 总数
{{ len .Alerts.Firing }}    # firing 数
{{ len .Alerts.Resolved }}  # resolved 数
{{ range .Alerts.Firing }}  # 只遍历 firing
```

## 六、可用函数

### 6.1 Go text/template 内置

- `and` / `or` / `not` / `eq` / `ne` / `lt` / `le` / `gt` / `ge`
- `len` / `index` / `printf` / `print` / `println`
- `range` / `if` / `else` / `with` / `define` / `template`

### 6.2 Alertmanager / Grafana 自定义

| 函数 | 例子 | 说明 |
|------|------|------|
| `toUpper` | `{{ "firing" \| toUpper }}` → `FIRING` | 大写 |
| `toLower` | 类似 | 小写 |
| `title` | `{{ "hello world" \| title }}` → `Hello World` | 首字母大写 |
| `trimSpace` | 去首尾空格 | |
| `join` | `{{ .Names \| join "," }}` | slice 连接 |
| `match` | `{{ match "^foo" "foobar" }}` → `true` | 正则匹配 |
| `reReplaceAll` | `{{ reReplaceAll "a" "b" "aaa" }}` → `bbb` | 正则替换 |
| `humanize` | `{{ .Values.A \| humanize }}` → `1.23k` | 数字人类可读 |
| `humanizePercentage` | `0.0123` → `1.23%` | 百分比 |
| `humanizeDuration` | 秒数 → `3h4m5s` | 时长 |
| `humanizeTimestamp` | Unix 秒 → 人类时间 | |
| `toTime` | 字符串 → time.Time | |
| `safeHtml` | 标记不转义 HTML | |
| `stripPort` | `host:port` → `host` | |

### 6.3 time.Time 的方法（用在 `.StartsAt` `.EndsAt`）

```
{{ .StartsAt.Format "2006-01-02 15:04:05" }}       # 格式化
{{ .StartsAt.Format "2006-01-02 15:04:05 MST" }}    # 带时区
{{ .StartsAt.Unix }}                                # Unix 秒
{{ .StartsAt.UTC }}                                 # 转 UTC
{{ humanizeDuration (sub now.Unix .StartsAt.Unix) }}  # 持续时长
```

## 七、Provisioning YAML 陷阱

### 7.1 `$` 转义

Grafana YAML provisioning 对 `$xxx` 做**环境变量替换**。未定义的 env var → 空串。

但 Alertmanager template 的范围变量就是 `$xxx`（如果使用 Go-style），所以：
- 用 `$$xxx` 在 YAML 里转义为字面 `$xxx`
- 但**我们现在不用 `$k $v` 风格**（Alertmanager 不支持 map 解包），所以其实不会触发这个问题
- 只有在模板里写了 `$foo` 形式的文本（非模板语法）时才需要考虑

### 7.2 YAML 字符串 vs 多行

推荐用 `|` 字面块保留换行：

```yaml
message: |
  第一行
  第二行 {{ .Status }}
```

**不要**用 `>`（会把换行折叠成空格）。

### 7.3 长模板易错字符

- `{{-` / `-}}` 去左/右空白（慎用，容易把该留的空格吃掉）
- 反引号在 YAML 字符串里要小心，建议改用普通文本
- 中文冒号 `：` 是合法字符，不影响模板解析

## 八、完整 Example（按原样复制即可用）

```yaml
apiVersion: 1

# ⚠️ 改动后必须重启 Grafana（contactpoints 不被 alerting reload API 刷新）
# ⚠️ 用 Alertmanager template，不是完整 Go template——map 必须用 .SortedPairs 迭代
contactPoints:
  - orgId: 1
    name: webook-email
    receivers:
      - uid: webook-email
        type: email
        disableResolveMessage: false       # 告警恢复时也发通知
        settings:
          addresses: "ops@company.com;manager@company.com"   # 多个用 ; 或 , 分隔
          singleEmail: false                                  # false = 每个收件人单独一封
          # ── 主题 ────────────────────────────────────────
          subject: '[{{ .CommonLabels.env | toUpper }}][{{ .Status | toUpper }}:{{ .CommonLabels.severity }}] {{ .GroupLabels.alertname }} ({{ len .Alerts.Firing }}F/{{ len .Alerts.Resolved }}R)'
          # ── 正文 ────────────────────────────────────────
          message: |
            {{- /* 顶部摘要 */ -}}
            状态：{{ .Status }}
            告警组：{{ .GroupLabels.alertname }}
            时间：{{ (index .Alerts 0).StartsAt.Format "2006-01-02 15:04:05 MST" }}
            Firing：{{ len .Alerts.Firing }}
            Resolved：{{ len .Alerts.Resolved }}

            {{- /* 组维度 labels */ -}}
            ── 分组标签 ──────────────────────────────
            {{ range .GroupLabels.SortedPairs }}{{ .Name }}={{ .Value }}
            {{ end }}

            {{- /* 共同 annotation（所有告警共享的） */ -}}
            ── 共同备注 ──────────────────────────────
            {{ range .CommonAnnotations.SortedPairs }}{{ .Name }}：{{ .Value }}
            {{ end }}

            {{- /* 逐条告警详情 */ -}}
            ══════════════════════════════════════════
            ━━━ Firing ({{ len .Alerts.Firing }}) ━━━
            {{ range .Alerts.Firing }}
            【{{ .Labels.alertname }}】{{ .Annotations.summary }}
            ├─ 开始：{{ .StartsAt.Format "2006-01-02 15:04:05" }}（持续 {{ humanizeDuration (sub now.Unix .StartsAt.Unix) }}）
            ├─ 严重：{{ .Labels.severity }}
            ├─ 团队：{{ .Labels.team }}
            ├─ 实例：{{ .Labels.instance }}
            ├─ 指纹：{{ .Fingerprint }}
            ├─ 值：{{ .ValueString }}
            ├─ Runbook：{{ .Annotations.runbook_url }}
            ├─ 规则详情：{{ .GeneratorURL }}
            └─ 静默：{{ .SilenceURL }}
            {{ end }}

            {{ if .Alerts.Resolved -}}
            ━━━ Resolved ({{ len .Alerts.Resolved }}) ━━━
            {{ range .Alerts.Resolved }}
            ✓ {{ .Labels.alertname }} 已恢复
            ├─ 开始：{{ .StartsAt.Format "2006-01-02 15:04:05" }}
            ├─ 结束：{{ .EndsAt.Format "2006-01-02 15:04:05" }}
            └─ 持续：{{ humanizeDuration (sub .EndsAt.Unix .StartsAt.Unix) }}
            {{ end }}
            {{- end }}

            ══════════════════════════════════════════
            Grafana：{{ .ExternalURL }}
```

## 九、常见错误 cheatsheet

| 报错 | 原因 | 修复 |
|------|------|------|
| `can't evaluate field StartsAt in type *templates.ExtendedData` | `.StartsAt` 写在 top-level（不在 `range .Alerts` 里） | 移到 range 内 |
| `unexpected "," in range` | 用了 `{{ range $k, $v := map }}`，Alertmanager template 不支持 | 改 `.SortedPairs` |
| template 执行到 `<.StartsAt>` 但变量空 | YAML `$` 被当 env var 吃掉（老版本问题） | `$` 写 `$$`（现在用 SortedPairs 之后不太会遇到） |
| `error calling humanizePercentage: can't convert <nil> to float` | `.Values.X` 某个 refId 没值（NoData 状态） | 包 `{{ if .Values.X }}...{{ end }}` |
| 模板改了没生效 | `alerting/reload` API 只刷 rules，不刷 contactpoints | **重启 Grafana**：`docker compose restart grafana` |
| 邮件主题乱码 | SMTP from_name 含中文但 encoding 不对 | 主题用 ASCII，正文用 UTF-8 |

## 十、调试建议

### 10.1 看 Grafana 实际加载的模板

```bash
curl -sS -u admin:admin http://grafana:3001/api/v1/provisioning/contact-points | jq
```

这个 API 返回的 `settings.message` / `settings.subject` 字段就是 Grafana 真正用到的模板（已做过 `$` 替换）。如果你写的 `$k` 被吃了这里就能看到。

### 10.2 打开 debug 日志看模板渲染错误

```bash
docker exec webook-grafana sh -c 'cat >> /etc/grafana/grafana.ini <<EOF
[log]
level = debug
filters = alerting.notifier:debug
EOF'
docker compose restart grafana

docker logs -f webook-grafana | grep -iE 'template|email|smtp'
```

### 10.3 Grafana UI 的模板预览

UI 里（如果不是 provisioned）：**Alerting → Contact Points → Edit → Test** 能直接预览渲染结果。
Provisioned 的 contact point UI 只读，可以临时手动新建一个测试版做预览。

## 十一、参考链接

- Alertmanager 模板字段：<https://prometheus.io/docs/alerting/latest/notifications/#data-structures>
- Alertmanager 默认模板源码：<https://github.com/prometheus/alertmanager/blob/main/template/default.tmpl>
- Grafana 告警模板扩展：<https://grafana.com/docs/grafana/latest/alerting/configure-notifications/template-notifications/reference/>
- Grafana 告警模板函数：<https://grafana.com/docs/grafana/latest/alerting/configure-notifications/template-notifications/language/>
