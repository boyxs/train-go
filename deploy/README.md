# L1 部署 - 一份 compose + env 切换

项目**唯一的部署真相源**。根目录的 docker-compose.yaml / nginx/ / prometheus/ / grafana/ / otel-collector/ 已经全部合并到这里。将来 K8s 等并列新开目录（`k8s/` 或 `helm/`）。

## 四种启动方式

| 模式 | 镜像来源 | 宿主端口 | 适合 |
|------|---------|---------|------|
| `./deploy.sh local` | 本地 build（override） | 中间件都暴露 | Windows 开发机，go run / DBeaver 连中间件 |
| `./deploy.sh dev` | ghcr pull master-latest | 只暴露监控栈 | 团队共享 dev 服务器 |
| `./deploy.sh staging` | 同上 | 端口和 dev 错开 | 预发布 |
| `./deploy.sh prod` | ghcr pull 语义化版本 | 端口再错开 | 生产 |

**同时只能跑一个**（container_name 全局唯一天然阻止），切换时 deploy.sh 自动 stop 旧的。volume 按 project 独立保留（`webook-<env>_mysql-data` 等），切回来数据还在。

## 一切都在 `.env.<env>` 里

```
deploy/.env.local / .env.dev / .env.staging / .env.prod
```

全部可调项都通过环境变量控制（compose 里都是 `${VAR:-default}` 带默认值）：

| 类别 | 变量 |
|------|------|
| 镜像 | `GH_USER`、`IMAGE_TAG`、`FE_IMAGE_TAG` |
| webook 配置 | `APP_ENV`（选哪份 yaml）、`DEPLOY_ENV` |
| 凭证 | `MYSQL_PASS` / `REDIS_PASS`（必须和 `webook/config/<env>.yaml` 里 `mysql.dsn` / `redis.password` 一致） |
| 业务/中间件内存 | `MYSQL_MEM`、`REDIS_MEM`、`KAFKA_MEM`、`KAFKA_HEAP`、`ES_MEM`、`ES_HEAP`、`FE_MEM`、`OLLAMA_MEM` |
| 监控栈内存 | `PROMETHEUS_MEM`、`GRAFANA_MEM`、`ZIPKIN_MEM`、`ZIPKIN_HEAP`、`OTEL_MEM`、`NGINX_MEM` |
| 宿主端口 | `NGINX_PORT`、`PROMETHEUS_PORT`、`GRAFANA_PORT`、`ZIPKIN_PORT`（四份 env 错开以免多 env 同时跑冲突） |
| 宿主端口（local only） | `WEBOOK_HOST_PORT`、`MYSQL_HOST_PORT`、`REDIS_HOST_PORT`、`ETCD_HOST_PORT`、`KAFKA_HOST_PORT`、`ES_HOST_PORT`、`OLLAMA_HOST_PORT`、`OTEL_HOST_PORT` |
| Grafana | `GRAFANA_USER`、`GRAFANA_PASS`、`GRAFANA_SMTP_*`、`ALERT_EMAIL_TO` |

## 首次部署

```bash
# 本地
cd deploy
./deploy.sh local                # 自动 build + up

# 服务器
scp -r deploy/ user@server:~/webook-deploy/
ssh user@server
cd ~/webook-deploy
chmod +x deploy.sh
echo "ghp_xxx" | docker login ghcr.io -u YOUR-GH-USER --password-stdin
vim .env.prod                    # 至少改 GH_USER + 上线前改强密码
./deploy.sh prod
```

## 日常操作

```bash
./deploy.sh local                          # 本地 build + up
./deploy.sh dev                            # 切 dev，自动 stop local
./deploy.sh prod 1.0.1                     # prod 指定 tag（会写回 .env.prod）
./deploy.sh dev logs [service]             # 日志（默认 webook）
./deploy.sh dev status                     # ps
./deploy.sh dev pull                       # 只拉镜像（local 用 build）
./deploy.sh dev build                      # 只 build（local）
./deploy.sh dev restart webook             # 重启某服务
./deploy.sh dev down                       # 停（volume 保留）
./deploy.sh dev nuke                       # 停 + 清 volume（prod 需确认）
./deploy.sh list                           # 所有 env 残留总览

# 启用 LLM（ollama）
COMPOSE_PROFILES=llm ./deploy.sh dev
```

## 文件布局

```
deploy/
├── docker-compose.yaml              # 基础定义（17 服务）
├── docker-compose.local.yaml        # local override：build + 暴露宿主端口
├── .env.local(.example)             # 四份 env 切换
├── .env.dev(.example)
├── .env.staging(.example)
├── .env.prod(.example)
├── deploy.sh                        # ./deploy.sh <local|dev|staging|prod>
├── nginx/
│   ├── nginx.conf
│   └── conf.d/default.conf
├── prometheus/
│   ├── prometheus.yml
│   ├── prometheus.local.yml           # 本地独立 prometheus.exe 跑时用（targets localhost:8089，只采集 webook 应用）
│   ├── alerting/                      # 告警规则（可选填）
│   └── examples/                      # 参考示例（不加载）
├── grafana/
│   ├── provisioning/                  # Grafana 自动装载
│   │   ├── datasources/               # prometheus + zipkin
│   │   ├── dashboards/                # 大盘 JSON
│   │   └── alerting/                  # contactpoints / policies / rules
│   ├── examples/                      # 参考示例（不装载）
│   └── Makefile                       # reload / test-email / restart 等运维命令
├── otel-collector/config.yaml
└── README.md
```

## webook 应用配置 yaml 对应关系

镜像里打包 4 份 yaml：`webook/config/{local,dev,staging,prod}.yaml`。

| .env.<env> | APP_ENV | webook 加载 | 用途 |
|-----------|---------|-----------|------|
| `.env.local` | `config/dev.yaml` | 容器里走 service name DNS | 本地 Docker 部署 |
| `.env.dev` | `config/dev.yaml` | 同上 | 团队 dev 服务器 |
| `.env.staging` | `config/staging.yaml` | 同上 | 预发 |
| `.env.prod` | `config/prod.yaml` | 同上 | 生产 |

Windows 上 `go run main.go` 时 `APP_ENV=config/local.yaml`（localhost 地址），**不走这里**——IDE 直连 host 端口。

## 为什么 K8s 不挤进来

`deploy/` 是 docker-compose 的世界。K8s 的 Helm chart / manifest 将来新开 `k8s/` 或 `helm/` 目录与之并列，不混在一起。迁到 K8s 时：

| L1 这里 | K8s 等价 |
|---------|---------|
| compose project `webook-<env>` | namespace `<env>` |
| `.env.<env>` | Secret + ConfigMap |
| compose `environment:` 注入 | `envFrom.secretRef` |
| Viper AutomaticEnv（已预埋） | 同 |
| container_name 冲突阻止并发 | namespace 隔离，天然可并发 |

详见 `C:\Go\notes\cicd-webook-roadmap.md`。
