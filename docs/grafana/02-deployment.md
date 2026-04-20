# 部署与配置

## 一、当前部署（deploy/docker-compose.yaml）

> 2026-04-20 起部署配置统一到 `deploy/` 目录。下面 compose 片段里的 `./grafana/...`
> 相对路径基于 `deploy/` 当前目录生效，结构等价迁移。

`deploy/docker-compose.yaml`：

```yaml
grafana:
  image: grafana/grafana:11.6.0
  container_name: webook-grafana
  restart: always
  mem_limit: 256m
  ports:
    - "3001:3000"          # 宿主机 3001 → 容器 3000
  environment:
    GF_SECURITY_ADMIN_USER: admin
    GF_SECURITY_ADMIN_PASSWORD: admin
  volumes:
    - ./grafana/provisioning:/etc/grafana/provisioning
    - grafana-data:/var/lib/grafana
  networks:
    extnetwork:
      ipv4_address: 172.21.0.21
```

**目录结构**：

```
deploy/grafana/                # 项目路径 work/deploy/grafana
└── provisioning/
    ├── datasources/
    │   └── prometheus.yml          # Prometheus 数据源
    └── dashboards/
        ├── dashboards.yml          # Dashboard provider 配置
        ├── README.md               # 推荐模板清单
        ├── linux-host.json         # 宿主机面板
        ├── webook-ops.json         # webook 运维面板
        └── webook-overview.json    # webook 总览面板
```

## 二、关键启动配置

Grafana 的所有配置项都可以用环境变量覆盖（格式：`GF_<SECTION>_<KEY>`，全大写）。

### 安全相关（生产必改）

```yaml
environment:
  # 默认账号（生产换强密码或走 SSO）
  GF_SECURITY_ADMIN_USER: admin
  GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_ADMIN_PASSWORD}

  # 禁止注册（社区版默认开放，生产必关）
  GF_USERS_ALLOW_SIGN_UP: "false"

  # 禁止匿名访问
  GF_AUTH_ANONYMOUS_ENABLED: "false"

  # Cookie secure（HTTPS 后开）
  GF_SECURITY_COOKIE_SECURE: "true"

  # 防止 dashboard 嵌入到外部网页（点击劫持防护）
  GF_SECURITY_ALLOW_EMBEDDING: "false"
```

### 域名 / HTTPS

```yaml
GF_SERVER_DOMAIN: grafana.webook.com
GF_SERVER_ROOT_URL: "https://grafana.webook.com"
GF_SERVER_PROTOCOL: https
GF_SERVER_CERT_FILE: /etc/grafana/cert.pem
GF_SERVER_CERT_KEY:  /etc/grafana/cert.key
```

> 实际生产建议 **Grafana 跑 HTTP，前面挂 Nginx/Caddy/Traefik 做 HTTPS 终结**，证书管理与限流交给反代。

### 元数据库（生产必切）

默认用 SQLite，单文件，存 `/var/lib/grafana/grafana.db`。问题：
- 不支持多实例 HA
- 大并发写入容易锁
- 备份只能停服或文件复制

**生产切 MySQL/Postgres**：

```yaml
GF_DATABASE_TYPE: mysql
GF_DATABASE_HOST: mysql:3306
GF_DATABASE_NAME: grafana
GF_DATABASE_USER: grafana
GF_DATABASE_PASSWORD: ${DB_PASSWORD}
```

需要先建库 + 用户：

```sql
CREATE DATABASE grafana CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'grafana'@'%' IDENTIFIED BY 'xxx';
GRANT ALL ON grafana.* TO 'grafana'@'%';
```

### 日志

```yaml
GF_LOG_MODE: console file
GF_LOG_LEVEL: info             # debug 排查问题用
GF_LOG_FILTERS: alerting:debug # 单模块开 debug
```

## 三、Provisioning（配置即代码）

Provisioning 让 Grafana 在**启动时自动加载**数据源、Dashboard、Alert 规则、通知渠道。**生产环境必用**，禁止手动点点点。

### 加载路径

```
/etc/grafana/provisioning/
├── datasources/      # 数据源 YAML
├── dashboards/       # Dashboard 加载器 YAML（指向 JSON 目录）
├── alerting/         # 告警规则 / 联系点 / 路由 YAML
├── notifiers/        # （Legacy 告警的通知通道，v8+ 用 alerting/）
└── plugins/          # 应用插件配置
```

### 数据源 provisioning（webook 现状）

`provisioning/datasources/prometheus.yml`：

```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy                          # 浏览器→Grafana→DS（推荐，凭证不下发）
    url: http://webook-prometheus:9090     # 容器网络内地址
    isDefault: true
    editable: true                         # 生产建议 false（防误改）
```

**关键字段**：

| 字段 | 说明 |
|------|------|
| `access: proxy` vs `direct` | proxy 走 Grafana 后端（推荐，跨域 + 凭证安全）；direct 浏览器直连数据源（已 deprecated） |
| `editable` | UI 是否能改。生产 `false`，所有变更走 Git |
| `jsonData` | 数据源专属配置（超时、HTTP 方法、版本等）|
| `secureJsonData` | 敏感字段（token / password），存储时加密 |

### Dashboard provisioning

`provisioning/dashboards/dashboards.yml`：

```yaml
apiVersion: 1
providers:
  - name: 'default'
    orgId: 1
    folder: ''                                       # 顶层文件夹
    type: file
    disableDeletion: false                           # 生产 true（不允许 UI 删）
    editable: true                                   # 生产 false
    allowUiUpdates: true                             # 生产 false（UI 改了下次重启会丢）
    updateIntervalSeconds: 30                        # 多久扫一次目录
    options:
      path: /etc/grafana/provisioning/dashboards     # 扫描这里所有 .json
```

只要把 `.json` 文件丢进 `path`，Grafana 启动 + 周期扫描会自动加载/更新。

### 生产推荐配置

```yaml
disableDeletion: true        # 防误删
editable: false              # 防误改
allowUiUpdates: false        # UI 改动不回写文件，重启会丢，反而误导
```

UI 上看着"不能编辑"很烦，但这是**强制让所有 dashboard 变更走 Git PR** 的关键。详见 `06-production-workflow.md`。

## 四、持久化数据

| 路径 | 内容 | 必须持久化 |
|------|------|-----------|
| `/var/lib/grafana/grafana.db` | SQLite 元数据库（用户、UI 编辑过的 dashboard、告警状态） | ✅ |
| `/var/lib/grafana/plugins/` | 通过 UI 安装的插件 | ✅（如果用过 UI 装插件）|
| `/var/lib/grafana/png/` | 渲染的 PNG 图（Reporting） | 否 |

webook docker-compose 已配 `grafana-data` named volume，OK。

**切 MySQL/Postgres 后**，`/var/lib/grafana/grafana.db` 不再使用，但 `/var/lib/grafana` 仍要持久化（插件、临时文件）。

## 五、健康检查 / 自身监控

Grafana 自带 `/api/health` 和 `/metrics`：

```bash
# 健康检查
curl http://localhost:3001/api/health
# {"database":"ok","version":"11.6.0",...}

# 自身指标（Prometheus 格式）
curl http://localhost:3001/metrics
```

把 Grafana 自己加进 Prometheus scrape：

```yaml
# prometheus.yml
scrape_configs:
  - job_name: grafana
    static_configs:
      - targets: ['webook-grafana:3000']
```

**关键自身指标**：

| 指标 | 含义 |
|------|------|
| `grafana_http_request_duration_seconds` | API 请求延迟 |
| `grafana_alerting_rule_evaluations_total` | 告警规则评估次数 |
| `grafana_alerting_rule_evaluation_failures_total` | 告警规则评估失败 |
| `grafana_datasource_request_total` | 数据源请求次数（按 DS 名字分） |
| `grafana_database_conn_in_use` | 元数据库连接数 |

## 六、升级

### 版本策略

- **OSS 一年大版本**：v10 → v11 → v12...
- 小版本基本兼容，大版本看 Breaking Changes 文档
- webook 锁定 `grafana/grafana:11.6.0`，不要用 `latest`

### 升级步骤（小版本）

```bash
# 1. 备份元数据库（SQLite）
docker cp webook-grafana:/var/lib/grafana/grafana.db ./backup/grafana-$(date +%F).db

# 2. 备份 provisioning（Git 已经管了，跳过）

# 3. 改 image tag → docker compose up -d
docker compose pull grafana
docker compose up -d grafana

# 4. 看启动日志确认 migration 成功
docker logs webook-grafana | grep -i migration
```

### 升级步骤（大版本）

加一步：**先在 staging 跑一周再上 prod**，看变更日志重点关注：
- Alerting：v8 / v9 / v10 / v11 各有改动
- Datasource 插件：旧版插件可能不兼容
- Auth：SAML/OAuth 配置项改名
- Panel：旧 Panel 类型废弃（Singlestat → Stat 等）

详细升级指南见 https://grafana.com/docs/grafana/latest/upgrade-guide/

## 七、常见启动问题

| 现象 | 原因 | 解决 |
|------|------|------|
| 容器起不来，报 permission denied | volume 挂载目录权限 | `chown -R 472:472 ./grafana-data`（472 是 grafana 容器用户） |
| 登录不上，admin/admin 提示密码错 | 之前改过密码，环境变量改了不生效（DB 已存） | `docker exec -it webook-grafana grafana cli admin reset-admin-password newpass` |
| Dashboard 没自动出现 | dashboards.yml 路径错 / JSON 格式错 / `folder` 名拼错 | 看日志 `docker logs ... | grep -i provisioning` |
| 数据源连不上 | url 用了 `localhost`（容器内）/ `127.0.0.1` | 用容器名 `webook-prometheus:9090` |
| Provisioning 后 UI 上想改改不了 | `editable: false`、`allowUiUpdates: false` | 设计如此，改 YAML 重启 |
