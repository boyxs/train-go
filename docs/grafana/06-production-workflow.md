# 生产级流程

UI 拖拖点点能做出 dashboard，但**生产环境必须把所有 Grafana 资产纳入工程化管理**：版本化、审核、自动化部署、可回滚。本章给出一套可落地的完整流程。

## 一、核心理念：Dashboard as Code

```
谁拥有 dashboard？
  ❌ 谁创建谁拥有 → 离职 / 调岗后无人维护
  ✅ Git 仓库拥有 → 责任在 PR review，变更可追溯

dashboard 的"源代码"在哪？
  ❌ Grafana 元数据库 → 不可审计、不可 diff
  ✅ Git 仓库 → 每次变更一个 commit
```

**Grafana UI 退化为"预览/调试工具"**，所有持久化变更必须经 Git。

## 二、整体流程

```
┌─────────────────────────────────────────────────────────┐
│  开发者                                                  │
│  ├─ ① 在 dev Grafana UI 上调好 panel                    │
│  ├─ ② Settings → JSON Model → 复制 JSON                 │
│  ├─ ③ 粘到 ops repo grafana/dashboards/<name>.json      │
│  └─ ④ 提 PR                                              │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│  CI                                                      │
│  ├─ ⑤ Lint：JSON 合法、uid 唯一、datasource uid 存在   │
│  ├─ ⑥ 跑 dashboard linter（grafana/dashboard-linter）  │
│  └─ ⑦ 在临时 Grafana 实例 import + smoke test           │
└──────────────────────┬──────────────────────────────────┘
                       │ merge
                       ▼
┌─────────────────────────────────────────────────────────┐
│  CD                                                      │
│  ├─ ⑧ 推到 staging Grafana（rsync / kubectl / API）    │
│  ├─ ⑨ 烟测通过 → 推 prod                                │
│  └─ ⑩ Prod Grafana 通过 provisioning 30s 内热加载       │
└─────────────────────────────────────────────────────────┘
```

## 三、仓库结构（推荐）

独立 ops repo（或在主 repo 里开 `grafana/` 目录）：

```
ops/
└── grafana/
    ├── README.md
    ├── datasources/
    │   ├── prometheus.yml
    │   ├── loki.yml
    │   └── tempo.yml
    ├── dashboards/
    │   ├── webook-overview.json
    │   ├── webook-http.json
    │   ├── webook-db.json
    │   └── infra/
    │       ├── linux-host.json
    │       └── prometheus-self.json
    ├── alerting/
    │   ├── rules/
    │   │   ├── webook-http.yml
    │   │   ├── webook-runtime.yml
    │   │   └── infra.yml
    │   ├── contactpoints.yml
    │   ├── policies.yml
    │   └── templates.yml
    ├── plugins/
    │   └── plugins.yml
    └── env/
        ├── dev.env             # 环境差异变量
        ├── staging.env
        └── prod.env
```

**多环境差异处理**：
- `datasources/`、`dashboards/`、`alerting/` 三个目录**完全相同**（不同环境用同一份）
- 环境差异（数据源 URL、密码、告警通道 Webhook）放 `env/<env>.env`
- 部署脚本根据当前环境注入 `${VAR}`

## 四、环境分层

| 环境 | 用途 | 变更进入门槛 |
|------|------|-------------|
| **local** | 开发者笔记本，docker-compose | 随便改 |
| **dev** | 共享开发环境 | UI 可改，但保存到 Git 才生效 |
| **staging** | 预发，与 prod 配置一致 | 走 CI/CD |
| **prod** | 生产 | 走 CI/CD + 人工审批 |

**关键约束**：
- staging / prod 的 Grafana provisioning 必须 `editable: false`、`allowUiUpdates: false`、`disableDeletion: true`
- dev 可以放宽（`editable: true`），方便开发者实验

## 五、Provisioning 配置（生产版）

`provisioning/dashboards/dashboards.yml`：

```yaml
apiVersion: 1
providers:
  - name: 'webook'
    orgId: 1
    folder: 'Webook'
    folderUid: webook                    # 固定 uid，环境间一致
    type: file
    disableDeletion: true                # 关键：UI 删了重启会回来
    editable: false                      # 关键：UI 不能改
    allowUiUpdates: false                # 关键：UI 改了不会写回 JSON
    updateIntervalSeconds: 30
    options:
      path: /etc/grafana/provisioning/dashboards
      foldersFromFilesStructure: true    # 子目录变 Grafana folder

  - name: 'infra'
    orgId: 1
    folder: 'Infra'
    folderUid: infra
    type: file
    disableDeletion: true
    editable: false
    allowUiUpdates: false
    options:
      path: /etc/grafana/provisioning/dashboards/infra
```

`disableDeletion + editable: false + allowUiUpdates: false` 三件套必须同时打开，缺一个都会有人 UI 上偷偷改。

## 六、Dashboard JSON 规范

每份 dashboard JSON **入仓前必须满足**：

| 项 | 要求 |
|----|------|
| `uid` | 必填，**手动指定**（不要让 Grafana 随机生成），命名 `<project>-<scope>` 如 `webook-overview` |
| `title` | 必填，加项目前缀 `Webook / Overview` |
| `tags` | 必填，含项目名 / 环境用途 / 拥有团队 `["webook", "backend", "overview"]` |
| `timezone` | 推荐 `browser`（自适应用户时区） |
| `schemaVersion` | 跟 Grafana 版本走，不手动改 |
| `time` | 默认时间窗口，`now-1h to now` |
| `refresh` | 默认 `30s`（小于 10s 压数据源） |
| `version` | 必须删掉或重置（Git diff 噪音）|
| `id` | 必须删掉（不同环境分配的 id 不同，留着会冲突）|
| 数据源 | 用 uid 而不是 name，全部 `${DS_PROMETHEUS}` 或固定 uid |

### 自动清洗脚本

UI 导出的 JSON 含 `version` / `id` 等字段，入仓前用 `jq` 清洗：

```bash
jq 'del(.id, .version) | .uid = "webook-overview"' raw.json > dashboards/webook-overview.json
```

可以做成 `make` 目标。

## 七、CI 校验

`.github/workflows/grafana.yml`（示意）：

```yaml
name: Grafana CI
on:
  pull_request:
    paths: ['grafana/**']

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # 1. JSON 格式校验
      - name: jq parse
        run: |
          for f in grafana/dashboards/**/*.json; do
            jq empty "$f" || exit 1
          done

      # 2. uid 唯一性
      - name: check uid unique
        run: |
          jq -r '.uid' grafana/dashboards/**/*.json | sort | uniq -d | \
            grep . && { echo "duplicate uid"; exit 1; } || true

      # 3. dashboard-linter（社区工具）
      - name: dashboard-linter
        run: |
          go install github.com/grafana/dashboard-linter@latest
          for f in grafana/dashboards/**/*.json; do
            dashboard-linter lint "$f"
          done

      # 4. 在临时 Grafana 烟测
      - name: spin up grafana
        run: |
          docker run -d --name g \
            -v $PWD/grafana/provisioning:/etc/grafana/provisioning \
            -v $PWD/grafana/dashboards:/etc/grafana/provisioning/dashboards \
            grafana/grafana:11.6.0
          sleep 15
          # 看启动日志没有 provisioning 错误
          docker logs g 2>&1 | grep -iE "error|fail" | grep -i provisioning && exit 1 || true
```

**dashboard-linter 检查项**：

- panel 是否设了 unit
- 查询 legend 是否有用 `{{var}}`
- 是否用了已废弃的 panel 类型（Singlestat 等）
- 模板变量定义是否合理

## 八、CD 部署

### 方案 A：rsync + 容器重启（最简单）

```bash
# CI 在 main 分支 merge 后
rsync -av grafana/ ops-server:/opt/webook/grafana/
ssh ops-server 'cd /opt/webook && docker compose up -d --force-recreate grafana'
```

**优点**：简单
**缺点**：重启 Grafana 几秒不可用；UI 当前看图的人会闪断

### 方案 B：Provisioning 热加载（不重启）

Grafana provisioning **dashboard 目录每 30s 自动扫描**，rsync 完不用重启：

```bash
rsync -av grafana/dashboards/ ops-server:/opt/webook/grafana/dashboards/
# 30 秒内 Grafana 自动重新加载 dashboard
```

但 **datasource / alert / plugin 的 provisioning 需要 reload API 或重启**：

```bash
curl -X POST http://admin:pass@grafana/api/admin/provisioning/datasources/reload
curl -X POST http://admin:pass@grafana/api/admin/provisioning/alerting/reload
```

### 方案 C：Grafana HTTP API + Terraform / grizzly

更工程化的做法：

- **grafana/grizzly**：CLI 工具，把 Git 里的 YAML/JSON 同步到 Grafana 实例
- **Terraform grafana provider**：HashiCorp 风格，状态管理 + 计划 + 应用

**适合大规模**：dashboard 上百个、多 org、多环境时收益明显。webook 起步阶段用方案 B 即可。

### 方案 D：K8s + ConfigMap

K8s 部署时 dashboard JSON 放 ConfigMap，Grafana Pod mount：

```yaml
volumeMounts:
  - mountPath: /etc/grafana/provisioning/dashboards
    name: dashboards
volumes:
  - name: dashboards
    configMap:
      name: grafana-dashboards
```

更新 ConfigMap 后 Grafana 30s 内热加载（kubelet 同步过去）。

## 九、权限与团队

### 组织 / 团队 / 用户结构

```
Organization: webook
├── Team: backend
│   ├── liu.hongjun
│   └── ...
├── Team: data
└── Team: sre

Folder: Webook (perm: backend=Admin)
Folder: Infra (perm: sre=Admin, backend=Viewer)
Folder: Business (perm: data=Admin, backend=Editor)
```

### 角色

| 角色 | 权限 |
|------|------|
| **Viewer** | 看 |
| **Editor** | 看 + 创建/编辑/删除 dashboard |
| **Admin** | 上述 + 数据源 + 告警 + 用户 |
| **Server Admin** | 跨 org，最高权限 |

### 推荐策略

- 大部分人 Editor（dev 环境）/ Viewer（prod 环境）
- 关键 folder（Infra）只对 SRE Admin
- prod Grafana 的 Editor 也无意义（`editable: false` 改不动）→ 都给 Viewer

### SSO

生产强烈推荐 SSO（OAuth2 / SAML / LDAP），不要用本地账号。

```yaml
GF_AUTH_GENERIC_OAUTH_ENABLED: "true"
GF_AUTH_GENERIC_OAUTH_CLIENT_ID: ...
GF_AUTH_GENERIC_OAUTH_CLIENT_SECRET: ...
GF_AUTH_GENERIC_OAUTH_AUTH_URL: ...
GF_AUTH_GENERIC_OAUTH_TOKEN_URL: ...
GF_AUTH_GENERIC_OAUTH_API_URL: ...
GF_AUTH_GENERIC_OAUTH_ROLE_ATTRIBUTE_PATH: 'contains(groups[*], "sre") && "Admin" || "Viewer"'
```

## 十、备份与恢复

### 必备份内容

| 内容 | 频率 | 方式 |
|------|------|------|
| 元数据库（用户、UI 改动、告警状态、API key） | 每天 | mysqldump / pg_dump（SQLite 文件 cp） |
| Provisioning 配置 | Git 已管理 | / |
| Dashboard JSON | Git 已管理 | / |
| 插件目录 `/var/lib/grafana/plugins/` | 每周 | tar |

### SQLite 备份脚本示例

```bash
#!/bin/bash
# /opt/scripts/grafana-backup.sh
DATE=$(date +%F)
docker cp webook-grafana:/var/lib/grafana/grafana.db /backup/grafana/grafana-$DATE.db
# 7 天保留
find /backup/grafana -name "grafana-*.db" -mtime +7 -delete
```

cron：`0 3 * * * /opt/scripts/grafana-backup.sh`

### 恢复步骤

```bash
docker compose stop grafana
docker cp /backup/grafana/grafana-2026-04-18.db webook-grafana:/var/lib/grafana/grafana.db
docker compose start grafana
```

### 异地备份

`/backup/grafana` rsync 到对象存储（S3 / OSS / R2）。

## 十一、变更管理

每次 dashboard / 告警变更都对应一个 PR，commit message 规范：

```
feat(grafana): 新增 webook 业务大盘
fix(grafana/alerting): 错误率告警阈值 1% → 2%（详见 INC-2026-04-15 复盘）
chore(grafana): 升级到 11.7.0
refactor(grafana/dashboards): webook-overview 拆分为 overview + http
```

**重大变更**（新增高级别告警 / 删 dashboard）必须有审批 reviewer。

## 十二、Disaster Recovery（灾备）

### 全部丢失时如何重建

只要 **Git + 备份的元数据库**还在：

1. 启动新 Grafana 实例
2. 恢复元数据库（用户、API key、告警状态）
3. Git checkout provisioning + dashboards → 启动加载
4. **5 分钟内恢复到丢失前状态**

如果元数据库也没了：
- 用户和 API key 重建
- Dashboard 全部从 Git 自动加载
- 告警规则从 Git 自动加载
- 告警状态历史丢失（可接受）

**核心保障**：Git 是 Source of Truth，元数据库只存"运行时状态"。

## 十三、监控 Grafana 自身

把 Grafana 当成一个普通服务监控：

| 指标 | 含义 | 告警 |
|------|------|------|
| `up{job="grafana"}` | 进程存活 | == 0 critical |
| `grafana_database_conn_in_use / max` | DB 连接 | > 0.8 high |
| `grafana_alerting_rule_evaluation_failures_total` | 告警规则执行失败 | rate > 0 high |
| `grafana_http_request_duration_seconds` | API 延迟 | P99 > 5s medium |
| `grafana_datasource_request_total{code!~"2.."}` | 数据源请求失败 | rate > 0 medium |

**告警规则评估失败**特别要监控——意味着告警子系统本身出问题，"告警的告警"。

## 十四、上线检查清单（Production Readiness）

新 Grafana 实例上线前对照：

- [ ] image 版本固定（不用 latest）
- [ ] admin 密码已改，禁止匿名/注册
- [ ] HTTPS（前置反代或 Grafana 内）
- [ ] 元数据库切 MySQL/Postgres（如果多实例）
- [ ] Provisioning 三件套（disableDeletion / editable=false / allowUiUpdates=false）
- [ ] Datasource uid 固定，password 走环境变量
- [ ] 至少一份关键告警 + 通道测试通过
- [ ] 元数据库每日备份脚本部署
- [ ] Grafana 自身指标已被 Prometheus 抓取
- [ ] 至少一份关键 dashboard（webook-overview）就位
- [ ] SSO / 团队 / folder 权限配好
- [ ] 文档（runbook）准备：故障如何看图、如何静默告警、如何回滚 dashboard
- [ ] On-call 值班表 + Contact Point 联通
