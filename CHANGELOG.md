# CHANGELOG

<!-- 新功能前插在此，日期降序 -->

## [2026-04-21] v1.0.0 基础设施增强：端口避让 + ghcr 源切换 + 懒拉镜像

**变更内容**: 围绕 prod 正式发版 (`webook-v1.0.0` / `webook-fe-v1.0.0`) 做了三件部署基建的事：

1. **prometheus 宿主端口重排**：dev/staging/prod 从 9090/9091/9092 → 9090/9190/9290，与 `KAFKA_EXTERNAL_PORT`（9094/9194/9294）百位 tier 对齐；prod 原 9092 撞 Kafka 业界默认端口
2. **ghcr 镜像源可切换**：`.env.*` 加 `GHCR_REGISTRY`（默认 `ghcr.io`），`deploy.sh` 加 `--ghcr <host>` / `--ghcr=<host>` flag 单次覆盖（不改 env 文件）；配合 README 新增的阿里云 ACR 反代教程，为国内 prod 拉取慢提供方案
3. **up 不再自动拉镜像**：compose 17 个 service 全加 `pull_policy: missing`，`deploy.sh` 的 `up` 分支删掉 `$COMPOSE pull`——镜像缺失才拉，已有直接启动；想显式刷新走 `./deploy.sh <env> pull`

**影响范围**:
- `deploy/docker-compose.yaml`（webook/webook-fe image 路径变量化、17 service 加 pull_policy）
- `deploy/deploy.sh`（新增 `--ghcr` flag 解析、删 up 里的 auto pull）
- `deploy/.env.{local,dev,staging,prod}` + 4 份 `.example`（新增 `GHCR_REGISTRY`、改 `PROMETHEUS_PORT`）
- `deploy/README.md`（端口表 + ghcr 切换节 + 阿里云 ACR 反代节 + pull_policy 说明）

**技术决策**:
- **端口**：百位 tier 对齐 kafka 系，dev 保持社区默认 9090；Grafana → Prometheus 走容器内 `webook-prometheus:9090`，不受宿主端口改动影响
- **ghcr 源**：只做 ghcr，不碰 Docker Hub（后者走 daemon `registry-mirrors` 更标准）；CLI flag 用 shell export 覆盖 `--env-file`，优先级天然更高。**实测公共 pull-through 代理（nju 等）拉不了私有 GHCR package**（404 manifest unknown），因此仓内默认官方源；要用加速域需把 package 改 public，或走阿里云 ACR 反代（教程见 README）
- **pull_policy**：默认非强制 pull 提速启动；显式刷新和容器启动分离，心智清晰。local 保持 `$COMPOSE build`，和 `docker-compose.local.yaml` 里 `pull_policy: never` 一起保证永远用本地构建产物

**坑**:
- **同 tag 远端被重推**：`pull_policy: missing` 判定"镜像已存在"不拉，继续用旧镜像。v1.0.0 force-tag 场景必须 `./deploy.sh <env> pull` 显式刷新后再 up
- **防火墙/安全组**：服务器若放通过 9091/9092 入站，需改放通 9190/9290

**用法**:
```bash
# 按需切 ghcr 源（前提：package 已 public）
./deploy.sh prod --ghcr <mirror>
# 或改 .env.prod: GHCR_REGISTRY=<mirror>（永久）

# 同 tag 刷新要显式 pull 再 up
./deploy.sh prod pull && ./deploy.sh prod
```

**会话**: 260421-cicd-L1发版v1.0.0

## [2026-04-20] deploy/ 合并根目录残留文件 + Makefile 重命名

**变更内容**: 上一轮合并时遗漏的根目录内容完整搬进 deploy/：
- `prometheus/examples/` → `deploy/prometheus/examples/`
- `prometheus/prometheus.local.yml` → `deploy/prometheus/prometheus.local.yml`
- `grafana/examples/` → `deploy/grafana/examples/`
- `grafana/mk/grafana.mk` → `deploy/grafana/Makefile`（改名 + 上提一级目录）
- 根目录 `docker-compose.yaml` / `nginx/` / `prometheus/` / `grafana/` / `otel-collector/` 全部清空

**影响范围**:
- `deploy/prometheus/` / `deploy/grafana/`（新增文件）
- `deploy/grafana/Makefile`（从 mk/ 移出 + 重命名）
- `deploy/README.md`（文件清单完整化）

**技术决策**:
- 保留示例目录（`examples/`）：Grafana/Prometheus provisioning 只加载 `provisioning/` 和 `prometheus.yml`，examples/ 放旁边做教学参考不会干扰
- Makefile 放 grafana/ 根下而非 mk/ 子目录：`cd deploy/grafana && make <target>` 直接可用，路径更短
- prometheus.local.yml 保留：Windows 本地独立跑 prometheus.exe 时用（targets: localhost:8089），和 prometheus.yml（容器内用 service name）并存不冲突

**实机验证（192.168.150.101）**: `./deploy.sh dev` 全部 17 服务启动成功 (nginx/grafana/prometheus/zipkin/etc 各自暴露端口，业务容器走 service name DNS)

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] deploy/ 成为唯一真相源 + 全 env 化

**变更内容**:
1. **删除根目录的部署配置**，合并进 deploy/：
   - `/docker-compose.yaml` → `deploy/docker-compose.yaml`（合并有价值的注释和配置）
   - `/nginx/` / `/prometheus/` / `/grafana/` / `/otel-collector/` → 整目录删除（deploy/ 已有同名内容）

2. **新增 local 模式**：
   - `deploy/docker-compose.local.yaml`：override 文件，webook/fe 从 ghcr pull 改本地 build，中间件+OTel 暴露宿主端口
   - `deploy/.env.local(.example)`：本地开发用
   - `deploy.sh local`：自动叠加 override + build + up

3. **全面 env 化**（消除硬编码）：
   - 监控栈内存：`PROMETHEUS_MEM` / `GRAFANA_MEM` / `ZIPKIN_MEM` / `ZIPKIN_HEAP` / `OTEL_MEM` / `NGINX_MEM`
   - 宿主端口：`NGINX_PORT` / `PROMETHEUS_PORT` / `GRAFANA_PORT` / `ZIPKIN_PORT`（dev/staging/prod 错开）
   - local 模式特有：`WEBOOK_HOST_PORT` / `MYSQL_HOST_PORT` / `REDIS_HOST_PORT` 等中间件宿主端口
   - compose 里全部 `${VAR:-default}` 带默认值，.env 不设也能跑

4. **从根目录挪过来的注释**（教学价值的）：
   - Kafka：`KAFKA_AUTO_CREATE_TOPICS_ENABLE` 线上不要开；`KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR` 单节点必须 1；healthcheck 解释
   - Zipkin：full vs slim 对比；mem 存储 vs ES 存储切换指引
   - Nginx：1.26.x LTS 注释；/var/log/nginx 不挂的 log forwarding 设计
   - Prometheus：`--web.enable-lifecycle` 热重载；host.docker.internal 加 extra_hosts
   - Grafana：`GF_SMTP_STARTTLS_POLICY: MandatoryStartTLS`
   - OTel：contrib 版选择理由

**影响范围**:
- **删**：根 `/docker-compose.yaml` / `nginx/` / `prometheus/` / `grafana/` / `otel-collector/`
- **新建**：`deploy/docker-compose.local.yaml` / `deploy/.env.local(.example)`
- **改**：`deploy/docker-compose.yaml`（全 env 化 + 注释迁移）、`deploy/deploy.sh`（加 local 分支）、四份 `.env.<env>(.example)`（统一全变量）、`deploy/README.md`、`webook/CLAUDE.md`

**技术决策**:
- **dev/staging/prod 宿主端口错开**（80/81/82、9090/9091/9092 等）：container_name 冲突已阻止并发，但万一切换间隙短暂并行时不打架
- **local 用 config/dev.yaml**：local.yaml 是为 go run + localhost 设计的，容器内用不通；local 部署用 dev.yaml 最省事
- **docker-compose 的 `!reset null`**：清 base image 字段给 local override 用，是 compose v2.20+ 特性
- **.env 里默认值 vs compose `${VAR:-default}`**：两处都有默认，优先级 .env > compose default。.env 里写全是为了可读，compose default 是兜底

**待办**:
- 服务器 192.168.150.101 `git pull` 后 volume 名保持 `webook-dev_*`（无迁移成本），直接 `./deploy.sh dev` 即可

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 部署方案彻底简化：一份 compose + env 切换

**变更内容**: 推翻之前"业务/infra 拆 project + 跨 project extnetwork 共享 + 固定 IP 段"的复杂设计，回归最小可用方案：
- `deploy/docker-compose.yaml`：**一份** all-in-one（17 服务：webook/fe/中间件/监控栈/exporters/nginx）
- `deploy/.env.{dev,staging,prod}(.example)`：简化（去掉 IP_PREFIX / ENV_NAME，加 APP_ENV）
- `deploy/deploy.sh`：重写，`./deploy.sh <env>` 切换；`list` / `down` / `nuke` / `logs` / `status` / `pull` / `restart` 子命令；裸操作子命令拦截
- `deploy/nginx/conf.d/default.conf`：**一份**（通配 `server_name _`），替代原 4 份（common/dev/staging/prod/monitor）
- `deploy/prometheus/prometheus.yml`：简化（target 用 service name 而非跨 project 容器名）
- `deploy/grafana/provisioning/datasources/*.yml`：URL 改 service name
- `deploy/otel-collector/config.yaml`：zipkin endpoint 改 service name

### 删除
- `deploy/docker-compose.deploy.yml` / `docker-compose.infra.yml`（合并）
- `deploy/docker-compose.infra.yml`、`deploy-infra.sh`
- `deploy/.env.infra` / `.env.infra.example`（并入各 env）
- `deploy/nginx/conf.d/{00-common,10-dev,10-staging,10-prod,20-monitor}.conf`（合并）

### 多环境新机制
- **project 名按 env 分**（`webook-dev` / `webook-prod`）→ volume 按 `webook-<env>_mysql-data` 自然隔离
- **container_name 不加前缀**（`webook-app`、`webook-mysql`）→ docker 全局唯一，**同时只能跑一套**（切换前 deploy.sh 自动 stop 别的 env）
- **APP_ENV 由 `.env.<env>` 注入** → webook 加载对应 `config/<env>.yaml`
- **切换 env 数据保留**：webook-dev 切到 webook-prod 再切回来，dev 的 mysql 数据还在

**影响范围**:
- `deploy/**`（大重构，新建 1 / 删除 10 / 改写 8）
- `webook/CLAUDE.md`（环境说明小段更新）
- `CHANGELOG.md`（本条）

**技术决策**:
- **不做多环境同机并行**：故障域耦合、资源浪费、和真实生产不匹配；团队化多环境等 L2 K8s namespace
- **靠 container_name 冲突阻止并发**：简单粗暴正确；不用代码校验，compose 自己报错
- **切换不清 volume**：数据留着，随时切回来有历史；nuke 子命令显式清理
- **监控不跨 env 共享**：每切一次 env 监控栈重建；prometheus 数据单 env 独立不混淆
- **webook/config/{local,dev,staging,prod}.yaml 四份保留**：yaml 选一份运行 = 模拟多环境配置，教学价值足；敏感字段继续走 Viper AutomaticEnv 覆盖

**待办**:
- 服务器 192.168.150.101 迁移：`git pull` + `./deploy.sh dev --nuke` 用新脚本重起（volume 名 `webook-dev_*` 与旧 project 一致，无需迁移数据）
- roadmap 笔记 1.17 共享 infra 章节删除，替换为"L1 = 单机单环境切换"

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 阶段二 · 完整服务栈 + 共享 infra + 固定 IP

**变更内容**: `deploy/` 目录从 6 文件扩到 20+ 文件，按 roadmap 1.7~1.9 + "共享 infra" 增强模式落地：

### 业务栈（每环境独立）
- `deploy/docker-compose.deploy.yml` 重写：新增 `webook-fe`、4 个 exporters（mysqld/redis/kafka/node）、`webook-ollama`（profile `llm` 默认关闭）；所有服务接入外部 `extnetwork` 并用 `${IP_PREFIX}` 变量驱动固定 IP；业务容器不再直接暴露宿主端口（改由 infra nginx 反代）
- `.env.{dev,staging,prod}.example` 重写：新增 `IP_PREFIX=172.21.{1,2,3}` / `FE_IMAGE_TAG` / `FE_MEM` / `OLLAMA_MEM`

### 共享 infra（跨环境一份）
- `deploy/docker-compose.infra.yml`：新建，含 nginx（`.28`）+ prometheus（`.20`）+ grafana（`.21`）+ zipkin（`.26`）+ otel-collector（`.29`，带 alias `otel-collector` 让业务容器原短名可解析）
- `deploy/.env.infra.example`：新建，含 Grafana 凭证 + SMTP 凭证
- `deploy/deploy-infra.sh`：新建，支持 `up/down/status/logs/reload`（nginx + prometheus 热更）
- `deploy/nginx/`：从根目录 `nginx/` 迁移，新建 `conf.d/{dev,staging,prod}.conf` 按子域名路由（`<env>.webook.local`）+ `monitor.conf`（grafana/prometheus/zipkin 子域名）
- `deploy/prometheus/prometheus.yml`：新建，跨环境固定 IP scrape（三环境 × 四种 exporter + 业务应用），每个 target 带 `env` label
- `deploy/grafana/provisioning/`：从根目录复制 datasources / dashboards / alerting，datasources URL 改成 infra 固定 IP
- `deploy/otel-collector/config.yaml`：新建，Zipkin exporter 指向固定 IP `172.21.0.26:9411`

### 脚本与 CI
- `deploy/deploy.sh` 重写：加外部网络自动创建、支持 `--profile llm` 参数、保留 prod 发版语义化版本闸门
- `.github/workflows/webook-fe-ci.yml`：和 backend CI 对齐 3 处增量（`feature/**` glob、`master-latest` 滚动 tag、`paths-ignore '**.md'`）

### 文档
- `deploy/README.md` 大改：含架构图、IP 分配表、首次部署命令序列、日常操作、资源估算、子域名规划

**影响范围**:
- `deploy/**`（新增 15+ 文件 / 覆盖 5 个）
- `.github/workflows/webook-fe-ci.yml`（增量 3 处）
- `CHANGELOG.md`（本条）
- `C:\Go\notes\cicd-webook-roadmap.md`（待后续同步）

**技术决策**:
- **路由方式选子域名**（而非路径前缀）：子域名下前端零改动（`NEXT_PUBLIC_API_BASE_URL=/api` 全环境通用），nginx `server_name` 区分 upstream；路径前缀需要 Next.js `basePath` + 每环境独立构建前端镜像，成本过高
- **exporter 每环境独立**：符合 L1 "每 project 独立全套"哲学，infra 只放跨环境共享的服务，prometheus scrape 按固定 IP 写死（三环境 × 四种 = 12 target）
- **ollama 做成 `profiles: ["llm"]`**：默认不启（2G 内存太大），按需 `./deploy.sh <env> --profile llm` 拉起
- **固定 IP 而非容器名解析**：对齐根目录 `docker-compose.yaml` 风格，prometheus 配置可读性 + 稳定性更高；业务内部的容器连接仍用容器名（`webook-mysql:3306` 之类）保持简单
- **otel-collector 容器加 network alias** `otel-collector`：webook 镜像里 `config/prod.yaml` 写死了这个短名，alias 让跨 project 解析成功
- **nginx 子域名 `*.webook.local`**：本地测试改 `/etc/hosts` 即可；上线换真实域名只改 `server_name`

**待办**:
- 实机验证：scp 到 Linux 机器跑 `./deploy-infra.sh up` + `./deploy.sh prod`
- L2：K3s + ArgoCD + Helm（阶段一 AutomaticEnv 钩子已预埋，Secret 注入方案就绪）

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 真环境隔离：混合配置方案（yaml + AutomaticEnv secrets 注入）

**变更内容**: 基于 L1 AutomaticEnv 钩子实现团队场景的真环境配置隔离：
- **新建 `webook/config/dev.yaml`** / **`staging.yaml`**：同构 prod.yaml，按环境差异化 `otel.env` / `sampleRatio` / logger 级别
- **`webook/config/prod.yaml`**：`mysql.dsn` / `redis.password` 改占位 `OVERRIDE_VIA_ENV`（敏感值不再硬编码）
- **`deploy/docker-compose.deploy.yml`**：`APP_ENV: "config/${ENV_NAME}.yaml"` 动态选 yaml；webook service 加 `MYSQL_DSN` / `REDIS_PASSWORD` env 注入
- **`.env.<env>.example` / `.env.<env>`**：加 `MYSQL_DSN` / `REDIS_PASSWORD` 覆盖变量
- `webook/CLAUDE.md` 环境说明重写，展示四段 yaml + 混合方案
- `deploy/README.md` 新增"混合配置方案"小节 + 密码同步 checklist

**影响范围**:
- `webook/config/{dev,staging,prod}.yaml`（+2 新建 + 1 改）
- `deploy/docker-compose.deploy.yml`、`.env.{dev,staging,prod}.example`、真实 `.env.*`
- `webook/CLAUDE.md`、`deploy/README.md`、`CHANGELOG.md`

**技术决策**:
- **yaml 分环境**比纯 env 注入对团队更友好：PR review "改 dev 配置" 直接看 dev.yaml diff；微服务 N 个服务 × M 环境不用维护 N×M 组 env 变量
- **敏感字段（密码）走 env 覆盖**：yaml 不进 git 敏感信息；`.env.*` 真实文件 gitignored
- **LLM API key 留 yaml**：`llm.providers` 是 slice-of-struct，Viper AutomaticEnv 不支持数组元素覆盖；学习场景全环境共用可接受；L2 真分离走 K8s Secret + JSON string 注入或重构为 map 结构
- **密码必须两处同步**（`MYSQL_PASS` 和 `MYSQL_DSN` 里嵌的密码）：接受手动一致性成本，换取 .env 无变量展开的简单性
- **1:1 对齐 L2 K8s**：yaml → ConfigMap、.env → Secret、docker-compose environment → envFrom.secretRef，应用代码零改动

**待办**: prod 上线前执行密码同步 checklist（deploy/README.md 列了 6 步）

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 阶段二：IP 引用改容器名，对齐 K8s 心智模型

**变更内容**: nginx / prometheus / grafana / otel-collector 的跨服务引用从固定 IP 改成容器名（`dev-webook-app:8089` / `webook-infra-prometheus:9090` 等），共 **30 处替换**。compose 层 `ipv4_address` 保留作为"网络规划底图"。

**影响范围**:
- `deploy/nginx/conf.d/10-{dev,staging,prod}.conf`（upstream app/fe 共 6 处）
- `deploy/nginx/conf.d/20-monitor.conf`（grafana/prometheus/zipkin proxy_pass 3 处）
- `deploy/prometheus/prometheus.yml`（15 个 env target + 3 个 infra target）
- `deploy/grafana/provisioning/datasources/{prometheus,zipkin}.yml`（2 处）
- `deploy/otel-collector/config.yaml`（zipkin exporter 1 处）
- `deploy/README.md`（新增"为什么引用用容器名"说明）

**技术决策**:
- **L2 心智对齐**：K8s 里没人手写 IP，全是 Service name DNS；L1 用容器名就是提前练这套思维。从思维训练角度比固定 IP 更有价值
- **维护成本**：改 IP 规划只动 compose 层 `ipv4_address`，监控/反代配置全不用改
- **底图保留**：`.env.<env>` 的 `IP_PREFIX` 和 compose 的 `ipv4_address` 不动，`tcpdump` / `iptables` 按段诊断仍可用
- **`allow 172.21.0.0/16`**：nginx metrics 端点的子网 allow 保留（子网过滤不是 host 引用）

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 阶段二 Hardening：DNS 隔离 + 密码占位 + Prometheus 认证

**变更内容**: 基于首轮 review 的 Important/Suggestion 修复：
- **网络隔离**：`deploy/docker-compose.deploy.yml` 每个 service 同时挂载 `default`（项目内 DNS）和 `extnetwork`（固定 IP 跨项目），避免三环境 `webook-mysql` service name 在共享外部网络上冲突
- **密码占位**：`.env.{dev,staging,prod,infra}.example` 中所有密码改 `CHANGE_ME`，仓库不再泄漏明文；真实 `.env.*` 保留有效值（gitignored）
- **Prometheus 配置修复**：`deploy/prometheus/prometheus.yml` 的 Grafana basic_auth 去掉 `${GRAFANA_PASS:-admin}` 占位（Prometheus YAML 不支持 env var 展开），改写死 `admin/admin` + 警告注释
- **nginx conf 顺序化**：`_common.conf` → `00-common.conf`，环境 conf 加 `10-`，监控加 `20-`，显式加载顺序不依赖下划线字母序
- **prometheus mem_limit**：256m → 512m，应对跨环境 × 多 exporter 的数据量
- **node-exporter 挂载注释**：说明 `/:/host:ro,rslave` 仅 Linux 生效
- **webook/main.go:41**：`initViper()`（V0 死代码）default env 从"local"回退"dev"，减少 review 噪音

**影响范围**:
- `deploy/docker-compose.deploy.yml`（每 service +1 `default` 网络，+1 网络声明）
- `deploy/docker-compose.infra.yml`（prometheus mem_limit）
- `deploy/prometheus/prometheus.yml`（Grafana basic_auth）
- `deploy/.env.{dev,staging,prod,infra}.example`（密码占位）
- `deploy/nginx/conf.d/*.conf`（5 个文件重命名加前缀）
- `deploy/README.md`（文件清单更新）
- `webook/main.go`（initViper 回退）

**技术决策**:
- **两网合一方案**：project 默认网络做 service-name DNS 隔离、extnetwork 做固定 IP 跨项目，两网各司其职；prometheus 走固定 IP 抓取（不靠 DNS），nginx 走固定 IP 反代（同理），应用内部连中间件走 service name DNS
- **Prometheus basic_auth 写死**：YAML 展开环境变量需要 Prometheus 2.45+ 的 `--enable-feature=expand-external-labels` 且语法受限，L1 阶段不值得引入；接受写死 + 文档提醒的妥协
- **example 用 `CHANGE_ME`**：强制部署者主动填密码，比示例写 `13520` 显得"可直接用"更安全

**待办**: 实机验证时确认两网方案的 compose v2 行为符合预期

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 阶段二 · 服务器部署模板落地（deploy/）

**变更内容**: 新建 `deploy/` 目录，按 roadmap 1.7~1.9 落地 L1 阶段二全部文件：
- `deploy/docker-compose.deploy.yml`：变量驱动的多环境部署清单（webook + MySQL + Redis + etcd + Kafka + ES），`-p webook-<env>` project 名隔离
- `deploy/.env.dev.example` / `.env.staging.example` / `.env.prod.example`：三环境变量模板，真实 `.env.<env>` 由 `.gitignore` 排除
- `deploy/deploy.sh`：一键部署脚本，按 env 参数拉镜像/启停/清理，prod 有额外闸门（`--down` 确认、`--nuke` 禁用、tag 必须语义化 x.y.z）
- `deploy/README.md`：使用说明 + 与根 `docker-compose.yaml` 的区别对照

**影响范围**:
- `deploy/*`（新目录 6 文件）
- `.gitignore`（加 `!.env.*.example` 允许模板入库）
- `CHANGELOG.md`（本条）

**技术决策**:
- `.env.*` 只入库 `.example` 模板，真实文件永远不进 git，避免密码泄露（即使本项目密码明文也是 13520）
- 服务器侧使用路径 `~/webook-deploy/`，但仓库内放在根目录 `deploy/`，避免和 `webook/`（后端代码）混在一起
- prod tag 强校验 `x.y.z` 语义化版本，倒逼走 `git tag webook-v*` 流程，不允许乱推 `master-<sha>` 上 prod
- 维持 L1 三环境同密码（13520）的限制，不在阶段二引入环境变量注入——那是 L2 的活

**待办**:
- L2：K3s + ArgoCD + Helm + Secret 分环境（webook/main.go 的 AutomaticEnv 钩子阶段一已预埋）
- 实机验证：需要一台 Linux 机器跑 `scp -r deploy/ user@server:~/webook-deploy/` → `./deploy.sh prod`

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] L1 阶段一 · CI 镜像流水线 + 配置命名标准化

**变更内容**:
1. **L1 阶段一（CI 打镜像闭环）**：
   - `webook/Dockerfile` 加 `ARG VERSION` + `LABEL org.opencontainers.image.version` + `ENV APP_ENV=config/prod.yaml`，CI 可注入版本元数据
   - `.github/workflows/webook-ci.yml` 增量补：`feature/**` 分支 glob、`master-latest` 滚动 tag、`VERSION` build-arg、`paths-ignore '**.md'`
   - `webook/main.go` `initViperV2` 加 `viper.AutomaticEnv()` + `SetEnvKeyReplacer('.', '_')`——**L2 钩子预埋**，现阶段零行为影响，未来 K8s Secret 经环境变量覆盖 yaml 时直接可用
2. **配置命名标准化（local/dev/prod 三段式）**：
   - `git mv webook/config/dev.yaml → config/local.yaml`，语义对齐"本地开发"实际用途
   - `config/dev.yaml` 本轮**不新建**，预留给未来 CI 集成测试 / 远端 dev 服务器
   - `main.go` 三个 initViper 版本的 default env 和 fallback 路径统一指向 `config/local.yaml`
   - `.vscode/launch.json` / `.idea/workspace.xml`（本地）/ IDE 环境变量指向 local.yaml
   - `local.yaml` / `prod.yaml` 尾部注释更新命名说明
   - `docs/opentelemetry/05-integration.md` 配置示例同步

**影响范围**:
- `webook/Dockerfile` / `.github/workflows/webook-ci.yml` / `webook/main.go`
- `webook/config/local.yaml`（新命名） / `webook/config/prod.yaml`（注释）
- `webook/CLAUDE.md`（加「环境说明」小节 + 启动命令示例）
- `docs/opentelemetry/05-integration.md`（配置示例）
- `C:\Go\notes\cicd-webook-roadmap.md`（外部笔记，补命名标准）

**技术决策**:
- 不走 C 方案的 `${WEBOOK_*}` + `os.ExpandEnv`——三份 yaml 各写各的值即是"环境适配"本身，环境变量占位对本项目无实际收益（roadmap 1.13 也印证这一点）
- L1 接受"三环境镜像里 yaml 同密码"的限制，真正分离等 L2 走 K8s Secret + `envFrom.secretRef` + Viper AutomaticEnv 链路
- `main.go` 的 V1/V0 是死代码但同步修改，保持一致防将来切换翻车
- `config/dev.yaml` 暂缓新建——没有真实使用者（CI 集成测试还没上）时建空配置徒增误导

**待办**:
- L1 阶段二：`~/webook-deploy/` 服务器侧 `docker-compose.deploy.yml` + `.env.<env>` + `deploy.sh`，等部署机就绪后做
- L2：K3s + ArgoCD + Helm + Secret 分环境，支持 prod 独立强密码
- CI 接入集成测试（需要 MySQL + Redis + Kafka + ES services，性能/成本另议）

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] GORM tracing 避开 AutoMigrate 启动噪音

**变更内容**: `webook/ioc/db.go` 把 `dao.InitTable(db)` 从 `tracing.NewPlugin` 注册之后调整到之前，让 AutoMigrate 的 `SELECT information_schema.statistics ...` 等系统表查询不再产生 `gorm.Query` / `gorm.Row` span。

**影响范围**:
- `webook/ioc/db.go`（仅调整 InitTable 与 tracing plugin 的注册顺序）

**技术决策**: gorm opentelemetry plugin 不支持按 database/table 过滤 span；`WithDBSystem` 只贴属性、不过滤。评估过 Collector 侧 filter processor（事后丢弃，浪费）、移除 Row callback（治标不治本）、换 uptrace/otelgorm（依赖变更大），最终选顺序调整——最小改动解决"启动期 information_schema 噪音"这个主要痛点。gormprom 保留在 InitTable 前，启动 SQL 仍会进 Prometheus metrics（聚合值影响忽略）。

**会话**: 260420-observability-采样率公式

## [2026-04-20] OTel Collector 升级 + 采样率公式文档化

**变更内容**: otel-collector 镜像 `0.88.0 → 0.105.0`；`webook/config/prod.yaml` 新增 `sampleRatio` 计算公式注释（`min(1.0, TargetTracesPerSec / AvgQPS)`）并在 `docs/opentelemetry/06-best-practices.md` 同步；`dev.yaml` 加公式引用注释。

**影响范围**:
- `docker-compose.yaml`（collector 版本）
- `otel-collector/config.yaml`（版本注释同步）
- `webook/config/prod.yaml` / `webook/config/dev.yaml`（采样率说明块）
- `docs/opentelemetry/06-best-practices.md`（新增「采样率计算公式」小节）

**技术决策**: 未选 0.116.0 因 CentOS 7 kernel 3.10 跑不起（`exec: no such file or directory`，实为新 Go runtime syscall 不兼容）；0.105.0 是经验证可运行的上限。采样率公式两处同步（配置 + 文档），注释明确标注"请勿删除"。

**待办**: 后续内核升级到 ≥5.x 或迁移 Rocky/Alma 9 后，可尝试升到 0.116+。

**会话**: 260420-observability-采样率公式

## [2026-04-19] 服务总览 dashboard + 监控栈自监控（Grafana/OTel/Zipkin）

**变更内容**: 新增 services-overview 大盘覆盖 up/主机/Go/MySQL/Redis/Kafka/监控栈/Zipkin 六大区；Prometheus 加 grafana/otel-collector/zipkin 三个 scrape job；OTel Collector 暴露 :8888 自监控；Zipkin 换 slim→full 以保留 /prometheus 端点

**影响范围**:
- `grafana/provisioning/dashboards/services-overview.json`（新）
- `prometheus/prometheus.yml`（+3 jobs，grafana 带 basic_auth）
- `otel-collector/config.yaml`（telemetry.metrics.address）
- `docker-compose.yaml`（zipkin 镜像换 full）

**技术决策**: grafana 显式带 basic_auth 兜底 metrics endpoint 可能 401；zipkin-slim 砍了 micrometer 所以换 full，+200MB 可接受

**会话**: 260419-ops-服务总览

## [2026-04-19] 配置模板参考体系 + Grafana/Prometheus examples 目录

**变更内容**: 为 `grafana/` 和 `prometheus/` 下每种配置文件建立完整注释的 `-example.yml` 模板 + 两份字段字典文档；Prometheus 启用 `--web.enable-lifecycle` 支持热 reload；Grafana dashboards 开启生产三件套

**影响范围**:
- 新增 `grafana/examples/`（独立目录，不被 Grafana 加载）：
  - `alerting/{contactpoints,policies,rules}-example.yml`（4 种告警范式、Alertmanager 模板语法、路由 + matcher + 静默时段）
  - `dashboards/{dashboards-example.yml, webook-example.json}`（provider 生产三件套 + 8 种 Panel 骨架 + 变量 + gridPos）
  - `datasources/{prometheus,zipkin,mysql,loki}-example.yml`（exemplar 联动、tracesToMetrics/Logs、只读账号、derivedFields）
  - `README.md` 对照表 + 使用流程
- 新增 `prometheus/examples/`：
  - `prometheus-example.yml`（global/scrape_configs/alerting/relabel/remote_write/多种 SD）
  - `alerts-example.yml`（Prom 原生告警规则 + recording rules + 模板变量说明）
  - `README.md` 解释两种告警方式（Prom 原生 vs Grafana Alerting）对比
- 新增文档：
  - `docs/grafana/08-alerting-template-reference.md`（Contact Point 邮件模板全字段字典 + Alertmanager template 语法 + 陷阱 cheatsheet）
  - `docs/grafana/09-provisioning-reference.md`（datasources/dashboards/rules/policies/Makefile 字段字典，定位为参考不是模板）
- 文档索引：`docs/grafana/README.md` + `docs/prometheus/README.md` 加"即开即用 example"板块
- 生产三件套落地：`grafana/provisioning/dashboards/dashboards.yml` 三件套全关（`disableDeletion:true editable:false allowUiUpdates:false`），UI 只读防误改
- Prometheus reload API 开启：`docker-compose.yaml` prometheus 服务加 `--web.enable-lifecycle`，`curl -X POST /-/reload` 即可热加载规则
- 模板修复（上轮未完全收敛）：
  - `grafana/provisioning/alerting/contactpoints.yml` 改成 Alertmanager template 语法（`.SortedPairs` 代替 `range $k,$v`）
  - `grafana/provisioning/alerting/rules.yml` 所有规则补 Reduce 节点，不再 "invalid format of evaluation results"
  - `otel-collector/config.yaml` verbosity 改回 basic（调试结束，detailed 日志量过大）
- 新增 `grafana/mk/grafana.mk`：Makefile 封装 reload / test-email / restart 命令

**技术决策**:
- `-example.yml` 文件单独放 `examples/` 不放 provisioning 下：Grafana 按扩展名加载，避免"测试模板被当真实数据源"；`.example` 后缀也能达到同样效果但 `-example.yml` + 独立目录更直观
- 文档 vs example 双份存在的定位：文档是"字段字典 + 坑速查"，examples 是"复制即用"。都改时以 examples 为准，文档只更新字段释义
- dashboards 生产三件套全关：UI 改动被重启覆盖会产生"诡异现象"（改完又消失），强制 Git 化唯一 source of truth

**待办**: 无
**会话**: 260419-template-reference-体系建设

## [2026-04-19] OTel 调用链完整串联 + Kafka 连接重试 + Grafana tracing dashboard

**变更内容**: 两个关键 bug 修复让 OTel trace 真正成为一棵完整的树；Kafka producer 启动竞态修复；Grafana 增加 tracing 入口 dashboard + 文档同步

**影响范围**:
- Bug 1：HTTP span 与 DB/Redis span 断成独立 trace
  - 根因：Gin 1.11 `*gin.Context.Value()` 默认不 fallback 到 `c.Request.Context()`
  - 修：`webook/ioc/web.go` `server.ContextWithFallback = true`
  - 测：`webook/ioc/web_ctx_propagation_test.go` 单测复现 true/false 两种行为
- Bug 2：Kafka producer 降级为 NoopProducer
  - 根因：webook 启动早于 Kafka JVM ready，`InitSaramaSyncProducer` 一次性失败返回 nil
  - 修（代码）：`webook/ioc/kafka.go` 新增 `retryConnect` 泛型工具，指数退避重试 6 次（共约 63s）；Producer + Client 都用
  - 修（编排）：`docker-compose.yaml` Kafka 加 healthcheck + webook `depends_on.kafka.condition: service_healthy`
  - 测：`webook/ioc/kafka_test.go` 3 用例（成功/耗尽/指数 backoff）
- Grafana tracing dashboard：`grafana/provisioning/dashboards/webook-tracing.json`
  - 5 个 panel：Text 说明 + QPS/错误率/P99/Goroutine Stat + HTTP QPS by path + 分位趋势
  - 顶部 link 跳 Zipkin UI + Webook/Overview
- Collector 版本：`otel/opentelemetry-collector-contrib:0.116.0` → `0.88.0`（CentOS 7 kernel 3.10 兼容）
- 文档同步：
  - `docs/opentelemetry/05-integration.md`：从"集成蓝图"改成"已落地 + 4 大踩坑记录"
  - `docs/opentelemetry/README.md`：加生产接入状态图（5 层 instrumentation + OTLP → Collector → Zipkin）
  - `docs/opentelemetry/06-best-practices.md`：踩坑清单追加 #13-16（Gin fallback / 启动竞态 / Collector kernel / IBM sarama）
  - `grafana/provisioning/dashboards/README.md`：新增"项目自有 Dashboard"表格

**技术决策**:
- ContextWithFallback=true 是 Gin + OTel 必选，单测保护防回归
- Kafka 重试走双保险：compose 层解决启动顺序，代码层解决运行期抖动
- retryConnect 抽成泛型工具，Producer/Client 共享，单测独立验证
- Collector 降到 0.88.0 兼容老 kernel，直接转发 OTLP → Zipkin，业务代码零改动

**生产验证（SSH）**:
- `Container webook-kafka Healthy` → webook 等 Kafka healthy 才起
- `docker logs webook-app | grep kafka` 空 → 无连接失败
- Zipkin API `?serviceName=webook&spanName=/article/reader/page` → 19 个 span 完整树形
- `[SERVER] /article/reader/page` 根下挂 `eval/get/select published_article×2/set/hgetall×10/select interaction`

**待办**: Service 层手动埋点（Article.Publish / Chat.Send）；Prometheus exemplar（Histogram observe 附 trace_id 实现大盘跳 trace）

**会话**: 260419-otel-fix-ctx-propagation+kafka-retry

## [2026-04-18] OpenTelemetry 全链路接入 + Grafana 邮件告警

**变更内容**: webook 集成 OpenTelemetry（OTLP/gRPC → otel-collector → zipkin），覆盖 HTTP 入口（otelgin）/ SQL（otelgorm）/ Redis（redisotel）；Grafana 接 Zipkin 数据源 + 配置 5 条核心告警 + QQ 邮箱 SMTP 通知

**影响范围**:
- OTel SDK 初始化：`webook/ioc/otel.go`（OTLP/gRPC exporter + Resource + ParentBased(TraceIDRatioBased) sampler + BatchSpanProcessor + W3C TraceContext propagator + wire cleanup）
- Wire 注入：`webook/wire.go` 加 `ioc.InitOTel`，签名改为 `(App, func(), error)`；集成测试 setup wire 加 `noop.NewTracerProvider`
- HTTP：`webook/ioc/web.go` `InitMiddlewares` 加 `tp trace.TracerProvider` 参数，挂 `otelgin.Middleware("webook", WithTracerProvider(tp))` 紧随 Prometheus 之后
- DB：`webook/ioc/db.go` `db.Use(tracing.NewPlugin(WithoutMetrics(), WithoutQueryVariables()))`
- Redis：`webook/ioc/redis.go` `redisotel.InstrumentTracing(client)`
- Kafka：`webook/ioc/kafka.go` 暂保留 TODO 注释（otelsarama 未适配 IBM/sarama）
- main：`webook/main.go` 处理 wire cleanup，进程退出 flush span
- 配置：`webook/config/{dev,prod}.yaml` 加 `otel` 块（dev → `localhost:4317`，prod → `otel-collector:4317`）
- 依赖：`webook/go.mod` 加 OTel SDK v1.32.0 + otlptracegrpc v1.32.0 + otelgin v0.57.0 + gorm.io/plugin/opentelemetry v0.1.16 + redisotel/v9 v9.18.0
- Mock 重生：`make -f mk/mock.mk mockgen`（go-redis 因 redisotel 升级，旧 mock 缺新方法）
- Collector：`otel-collector/config.yaml` 新增（OTLP receiver + memory_limiter + batch processor + zipkin exporter）
- Compose：`docker-compose.yaml` 加 `otel-collector` 服务（image `otel/opentelemetry-collector:0.116.0`，IP 172.21.0.29，port 4317）
- Grafana 数据源：`grafana/provisioning/datasources/zipkin.yml` 新增（uid=zipkin，关联 prometheus）；`prometheus.yml` 加 uid=prometheus 固定
- Grafana 告警：`grafana/provisioning/alerting/{contactpoints,policies,rules}.yml` 三件套
  - rules：5 条核心告警（服务 up / 5xx 率 > 1% / P99 > 500ms / Goroutine > 10000 / MySQL 连接 > 80%）
  - policies：按 alertname+severity 分组，critical 立即发，其它攒批 5m
  - contactpoints：邮件，收件人 `3236447743@qq.com`
- Grafana SMTP：`docker-compose.yaml` grafana 服务加 `GF_SMTP_*` 环境变量（QQ 邮箱 587 STARTTLS），凭证从 `.env` 注入
- 凭证管理：`.env`（本地真实，已 .gitignore）+ `.env.example`（模板，可入库）；`.gitignore` 加 `.env` `.env.*` 规则

**技术决策**:
- Exporter 选 OTLP/gRPC（CNCF 标准）+ otel-collector 中转：换后端只改 collector 出口，业务代码零改动；未来加 Tempo/Datadog 多 fan-out 也只动 collector
- Collector 用 core 版（80MB）而非 contrib（350MB）：仅需 OTLP receiver + zipkin exporter，core 全覆盖
- otelgin 紧随 Prometheus 之后：让所有下游中间件 / handler 都在 root span 上下文里
- Init 走 wire `(T, func(), error)` cleanup pattern：与项目既有 wire 风格一致，BatchSpanProcessor 队列退出时 flush
- Sampler 用 `ParentBased(TraceIDRatioBased)`：跨服务时跟随上游决策，避免一条 trace 一半在一半丢
- Kafka 暂未接 OTel：otelsarama 未适配 IBM/sarama，等社区或自己写包装；当前 producer/consumer span 缺失，TODO 标记
- gormopentelemetry 用 `WithoutMetrics()`：与现有 gormprom 互斥避免重复采集
- gormopentelemetry 用 `WithoutQueryVariables()`：SQL 参数可能含 PII，默认隐藏

**部署步骤（VM 上）**:
1. `git pull` 同步代码
2. `cp .env.example .env && vi .env` 填 SMTP 授权码（或 scp 已填好的 .env 过去）
3. `docker pull otel/opentelemetry-collector:0.116.0`（或国内镜像 retag）
4. `docker compose build webook && docker compose up -d`
5. 验证 trace：`curl http://192.168.150.101/api/user/login` → Zipkin UI `:9411` 看到 root span + DB/Redis 子 span；Grafana `:3001` Explore 选 Zipkin 也能查
6. 验证告警邮件：手动触发（如 stop webook → up == 0）等 1m 后收 QQ 邮箱

**待办**:
- Phase 3：service 层手动埋点（Article.Publish / Chat.Send / User.Login 等关键路径加 attribute / RecordError）
- Kafka OTel：自写 sarama producer interceptor / consumer middleware 注入 trace context 到 Kafka header
- Grafana exemplar：Prometheus Histogram observe 时附 trace_id，大盘高延迟点直跳 Zipkin

**会话**: 260418-otel-Phase1to5全链路接入

## [2026-04-18] 前后端 CI 完善 + GHCR 镜像推送 + nginx 同源部署

**变更内容**: 补齐前端 CI；前后端 CI 均扩展 `build-push` job 推镜像到 GHCR；docker-compose 新增 webook-fe + nginx 同源反代，实现浏览器只见一个域名、零 CORS
**影响范围**:
- 前端 CI 新建：`.github/workflows/webook-fe-ci.yml`（eslint + tsc + next build；master/feature/webook-fe-v 三种 tag 推 `ghcr.io/<user>/webook-fe`）
- 后端 CI 扩展：`.github/workflows/webook-ci.yml` 加 `build-push` job，先 `go build -tags=k8s` 再 docker build（保留简单 Dockerfile 策略）；tag `webook-v*.*.*` 也触发
- 前端 Dockerfile：`webook-fe/Dockerfile` npm registry 改 `ARG NPM_REGISTRY` 可传参（默认官方源，CI 用默认，本地按需切 npmmirror）
- 部署栈：`docker-compose.yaml` 加 `webook-fe`（3000 仅 expose）+ `nginx`（80 对外，同源反代：`/` → 前端，`/api/*` → 后端）
- nginx 配置：`nginx/nginx.conf` + `nginx/conf.d/webook.conf`（upstream + 安全头 + healthz + 代理参数）
- Workflow 完善：两个 workflow 加 `workflow_dispatch`（可手动触发）和顶层 `permissions: contents: read`
**技术决策**:
- 同源部署：前端 `NEXT_PUBLIC_API_BASE_URL=/api`（相对路径）**一次构建多环境通用**，不用再为不同 API 地址构建多个镜像
- Workflow `paths` 过滤移除：否则 tag push 指向的 commit 若 diff 不含目标目录会静默不触发，发版失败
- 后端不改 Dockerfile 走简单单阶段：与项目既有决策一致（commit 36e8dbe 回归精简），构建责任在 CI 外部
- npmmirror 改成构建参数而非硬编码：CI 走官方源避免海外 runner 访问国内镜像站失败
**待办**: GitHub 仓库 Settings → Actions → Workflow permissions 勾选 "Read and write permissions" 否则 GHCR 推送 403；设置分支保护强制 PR + CI 绿
**会话**: 260418-infra-CI完善前端部署

## [2026-04-18] 学习沙箱归档到 sandbox/

**变更内容**: `work/` 顶层散落的 8 个独立 Go 学习模块统一归档到 `sandbox/`，主目录只剩项目核心（webook / webook-fe / 基础设施 / docs）
**影响范围**:
- 目录归并（git mv 保留历史）：`context/` `gin/` `gorm/` `mongodb/` `opentelemetry/` `sarama/` `syntax/` `wire/` → `sandbox/<name>/`
- 文档同步：`docs/opentelemetry/` 共 7 处 `opentelemetry/` 路径引用 → `sandbox/opentelemetry/`
**技术决策**:
- 命名 `sandbox/` 而非 `learning/`：中性专业，不带学生味
- 扁平结构（不按主题分组）：沙箱本来就孤立，分类反而增加查找深度
- 各沙箱保留独立 go.mod，依赖树不互相污染
**待办**: 无
**会话**: 260418-refactor-沙箱归档

## [2026-04-18] 学习沙箱：context + opentelemetry trace

**变更内容**: 新增两个独立 Go 学习沙箱
**影响范围**:
- `sandbox/context/`（独立模块 `context-demo`，演示 WithValue/WithCancel/WithTimeout/父子传导/反向隔离，5 个测试）
- `sandbox/opentelemetry/`（独立模块 `otel-demo`，OTel SDK v1.32.0，stdout + Zipkin 双 exporter 测试）
**技术决策**: 独立 go.mod 与主模块隔离，依赖树不互相污染；与 `sandbox/mongodb/` `sandbox/sarama/` `sandbox/gin/` `sandbox/gorm/` `sandbox/wire/` `sandbox/syntax/` 等已有学习沙箱风格一致
**待办**: 无
**会话**: 260418-learning-context+OTel

## [2026-04-18] 前端 Docker 部署 + Nginx 反代 + 基础设施目录顶层化

**变更内容**: webook-fe 多阶段 Docker 镜像（standalone + 非 root + healthcheck + OCI labels）；Nginx 反向代理（同源部署 `/api` 剥前缀、SSE 关 buffering、登录强限流、静态资源长缓存、安全响应头）；docker-compose.yaml / nginx / prometheus / grafana 全部上移到 `work/` 顶层，`webook/` 目录回归"纯后端代码"
**影响范围**:
- 前端镜像：`webook-fe/Dockerfile`（多阶段 deps→builder→runner，standalone 输出，非 root nextjs:1001，HEALTHCHECK，OCI 标签）+ `webook-fe/.dockerignore` + `webook-fe/next.config.ts`（`output: 'standalone'` + `poweredByHeader: false` + `compress: true`）
- 反代：`nginx/nginx.conf`（JSON access log、gzip、limit_req zone）+ `nginx/conf.d/webook.conf`（upstream + 安全头 + SSE 特殊路径 + metrics 内网白名单 + Next 静态长缓存）
- 编排：`docker-compose.yaml` 加 `webook-fe`（仅 expose 3000）+ `nginx`（80 对外，不挂日志卷以保留 `/dev/stdout` 软链让 docker logs 接管）；启动命令注释更新
- 目录顶层化（git mv，保留历史）：`webook/docker-compose.yaml` → `docker-compose.yaml`、`webook/prometheus/` → `prometheus/`、`webook/grafana/` → `grafana/`
- 文档同步：`docs/grafana/` `docs/prometheus/` 共 4 处路径引用从 `webook/grafana` → `grafana` 等
**技术决策**:
- 同源部署：`NEXT_PUBLIC_API_BASE_URL=/api`，构建一次到处部署，无 CORS，外暴露端口仅 80
- standalone 输出：镜像最小化，仅装运行时 node_modules
- nginx 在容器网络内部通过 upstream `webook:8089` / `webook-fe:3000` 拼接，无需暴露后端端口（保留 8089 暂供调试）
- 顶层化：`webook/` 不再承担"基础设施 + 后端"双重职责，`nginx/prometheus/grafana` 是跨前后端基础设施，与 `webook/` 平级更清晰
**待办**: 后端去掉 8089 公网端口（仅走 nginx）；TLS（443）+ HSTS；webook 接 OTel；Grafana provisioning 切 `editable=false` 三件套
**会话**: 260418-deploy-前端Docker部署

## [2026-04-18] Grafana / OpenTelemetry 文档 + Zipkin 接入

**变更内容**: 新增 Grafana / OpenTelemetry 详细学习文档（共 13 篇），docker-compose 引入 Zipkin 服务（暂用内存存储），原 `docs/monitoring/` 重命名为 `docs/prometheus/` 与新文档对齐
**影响范围**:
- 文档：`docs/opentelemetry/`（概念/Tracing 模型/上手/Exporter 选型/接入 webook/最佳实践，6 篇）
- 文档：`docs/grafana/`（概念/部署/数据源/Dashboard/告警/生产级流程/最佳实践，7 篇）
- 文档重命名：`docs/monitoring/` → `docs/prometheus/`（git mv 保留历史，同步修复 grafana/otel 文档内引用）
- 基础设施：`docker-compose.yaml` 加 `zipkin`（openzipkin/zipkin:3.4，9411，IP 172.21.0.26，mem 存储 + ES 切换注释占位 + healthcheck）
**技术决策**:
- 文档目录三件套对齐（prometheus / grafana / opentelemetry），命名一致便于检索
- Zipkin 暂内存存储，重启丢 trace 数据；切 ES 路径以注释保留（复用 `webook-es`），单节点 ES 必须 `ES_INDEX_REPLICAS=0`
**待办**: webook 接入 OTel（otelgin/otelgorm/otelredis/otelsarama）；接入后切 OTLP exporter；按需切 Zipkin ES 存储
**会话**: 260418-observability-OTel起步

## [2026-04-17] GitHub 迁移 + CI 体系

**变更内容**: 仓库从 gitee 迁至 GitHub（`github.com/boyxs/train-go`），Go 模块路径改为 `github.com/webook`，落地 GitHub Actions CI（goimports + vet + test），handler 测试适配 ginx.WrapReq HTTP 状态码映射
**影响范围**:
- 模块路径：`webook/go.mod` + 132 个 .go 文件 import 全替换（`gitee.com/train-cloud/geektime-basic-go` → `github.com/webook`）
- Import 规范：`goimports -local github.com/webook` 重排 111 个文件，本地包分组到第三组
- Makefile：`webook/Makefile` 加 `fmt` / `fmt-check` target，MODULE 从 go.mod 动态提取
- CI：`.github/workflows/webook-ci.yml`（lint-test：goimports 检查 + go vet + go test -race），actions/checkout@v5、setup-go@v6（消除 Node 20 警告）
- Test 修复：`internal/web/` 5 个 test 函数的 wantCode 按 ginx.WrapReq 状态码映射校正（Code 4→400、Code 5→500）；`internal/service/sms/tencent/` TestSend 无凭证时改 `t.Skip()`
- Remote：origin → GitHub，原 gitee 重命名为 `gitee` remote 保留
**技术决策**:
- 模块路径采用 `github.com/webook` 而非 `github.com/boyxs/train-go/webook`：短路径更简洁，私有项目不需要外部 `go get`
- CI 不依赖 make，直接调 goimports 二进制：降低 runner 环境耦合
- 当前只有 lint-test job，build-push 留到 L1 完整流程时加（需配合 Dockerfile 多阶段）
**待办**: Dockerfile 改多阶段 + CI 加 build-push → GHCR；打开 GitHub 仓库分支保护（master 强制 PR + CI 绿）
**会话**: 260417-infra-GitHub迁移

## [2026-04-14] Prometheus 监控链路

**变更内容**: HTTP 指标中间件 + Prometheus/Grafana/Exporter Docker 栈 + 3 个自定义 Grafana 面板 + PromQL 文档
**影响范围**:
- 中间件：`pkg/ginx/middleware/metrics/`（Builder 接口 + PrometheusBuilder 实现，4 种指标按需启用）
- 集成：`ioc/web.go`（/metrics 端点 + 中间件注册）
- Docker：`docker-compose.yaml`（prometheus/grafana/mysqld-exporter/redis-exporter/kafka-exporter/node-exporter）
- 配置：`prometheus/prometheus.yml`、`prometheus/prometheus.local.yml`、`grafana/provisioning/`
- 面板：`webook-overview.json`（17 面板）、`webook-ops.json`（15 面板）、`linux-host.json`（18 面板）
- 文档：`docs/monitoring/`（架构/部署/PromQL/实战查询/告警/最佳实践，6 篇）
- 目录整理：`docs/` PRD 文件移至 `prd/`，`docs/` 改为技术文档目录
- 配置同步：`config/prod.yaml` 补齐 llm/embedding/ollama 配置
**技术决策**: 中间件用 Builder 接口抽象，Prometheus 为具体实现，后续可换 OpenTelemetry；Histogram + Summary 同时开启便于学习对比，生产建议二选一
**待办**: 生产环境 API Key 移到密钥管理
**会话**: 260414-monitoring-Prometheus监控

## [2026-04-12] AI 回复操作栏（复制 + 点赞/点踩）

**变更内容**: AI 消息气泡底部增加操作栏，支持复制回复内容、点赞/点踩反馈
**影响范围**:
- 后端：`domain/chat.go`（Message +Feedback）· `dao/chat_message.go`（+Feedback 字段 + UpdateFeedback）· `repository/chat_message.go`（UpdateFeedback + 清缓存）· `service/chat.go`（SetFeedback 归属校验）· `web/chat.go`（/chat/message/feedback 路由）
- 前端：`types/chat.ts`（+feedback）· `api/chat.ts`（setMessageFeedback）· `hooks/useChat.ts`（乐观更新 setFeedback）· `views/chat/MessageBubble.tsx`（ActionBar 组件）· `ChatMessages/ChatBubble/index.tsx`（props 传递）
- 原型：`prd/chat/chat.pen`（06-消息反馈画板）· `prd/chat/prototypes/06-消息反馈.png`
- 测试：8 个集成测试（点赞/取消/点踩/无效值/无效ID/非属主/幂等/列表展示）
**技术决策**: feedback 字段直接加在 message 表（1:1 关系），不新建表；前端乐观更新用函数式 setState 避免闭包竞态
**待办**: 无
**会话**: 260412-chat-消息反馈

## [2026-04-11] Chat SSE 断线续传（Redis Stream）

**变更内容**: 用 Redis Stream 实现 SSE 断线续传，刷新/切换对话后从断点继续流式输出
**影响范围**:
- 后端：`pkg/streamer/`（EventStreamer 接口 + Redis Stream 实现）· `service/chat.go`（publishToStream 写入 + ReadStream/BlockReadStream 读取 + isGenerating 内存标记）· `web/chat.go`（ResumeStream GET SSE 端点 + pollStream + formatSSE）· `consts/cache.go`（ChatStreamPattern + ChatStreamTTL）
- 前端：`api/chat.ts`（resumeStream SSE 重连函数）· `hooks/useChat.ts`（buffer 管理 + isStale 守卫 + resumeStream 恢复）
**技术决策**:
- Redis Stream 做消息中转，`XREAD BLOCK` 零空转阻塞读
- Stream 生成完成后 5 分钟 EXPIRE，供断线重连
- 前端 `isStale()` 守卫所有异步回调，防止快速切换对话时数据串
- 有活跃 buffer 时 effect 直接 return，避免与 send() 竞态
**会话**: 260411-SSE-Redis-Stream

## [2026-04-11] 文章润色助手 + 原型配色统一 + Chat 生成不中断 + 阅读量展示

**变更内容**: 文章润色助手（LLM 同步调用 + 对比 Modal）；原型配色全量统一为 teal #0D9488；Chat 生成独立 context 不被刷新/切换中断，支持刷新后轮询恢复；阅读页互动栏对齐原型（独立卡片 + 点赞收藏按钮样式）；文章广场/作者列表新增阅读量展示；首页按钮对齐原型；Header logo 统一主色；代码块语法高亮
**影响范围**:
- 后端：`service/article_polish.go`（润色服务）· `web/article_polish.go`（Handler + 限流）· `service/ai/llm.go`（LLMClient.Chat 同步方法）· `service/ai/openai.go` + `failover.go` + `timeout_failover.go`（Chat 实现）· `service/chat.go`（独立 context + placeholder + onFlush + isGenerating + trySend 非阻塞）· `web/chat.go`（isGenerating 接口）· `web/jwt/handler.go`（CheckSession Redis 容错）· `domain/chat.go`（ArticleCard.Url）· `chat_tools.go`（工具结果带 url）
- 前端：`views/article/read.tsx`（互动栏重构）· `views/article/feed.tsx`（阅读量）· `views/article/list.tsx`（阅读量列 + 移动端卡片重构）· `views/article/edit.tsx`（AI 润色按钮 + PolishModal）· `views/home.tsx`（按钮对齐原型）· `views/chat/index.tsx`（Header 导航重构）· `views/chat/MessageBubble.tsx`（工具卡片下移 + 语法高亮）· `hooks/useChat.ts`（buffer 缓存 + 轮询恢复）· `components/layout/Header.tsx`（logo 主色）
- 原型：3 个 pen 文件配色统一 + PNG 重新导出 + PRD 更新
**技术决策**:
- Chat 生成用 `context.WithTimeout(context.Background(), 2min)` 独立于 HTTP 请求
- placeholder 消息预插入 DB + onFlush 每 2 秒更新，支持刷新后轮询看到进度
- trySend 加 default 分支，channel 满时不阻塞（浏览器断开后生成继续）
- isGenerating 用内存 sync.Map，不依赖 Redis
- 切换对话用 bufferRef 缓存 pending 消息，切回来立即恢复
- CheckSession Redis 出错时容错放行，解决间歇性点赞状态丢失
**待办**:
- Chat SSE 断线续传：用 Redis Stream 替代轮询，支持 Last-Event-ID 断点续传（为微服务拆分做准备）
- P1 文章标签/分类体系 → 意图识别 + 路由分发
- P2 用户反馈（点赞/点踩单条回复）
**会话**: 260411-原型实现-润色-Chat优化

## [2026-04-10] AI 点击埋点 + 数据看板 + Chat 工具调用修复

**变更内容**: 新增 AI 文章点击埋点全链路（记录 + 看板 + 缓存），修复 Chat Function Calling SSE 事件嵌套层级、工具结果刷新丢失问题，代码块语法高亮，命名规范统一
**影响范围**:
- 后端：`web/click_event.go`（Handler）· `service/click_event.go` · `repository/click_event.go` · `dao/click_event.go` · `cache/click_event.go`（全链路新增）· `service/chat.go`（saveReply 持久化 toolResults、buildPrompt 用 ListRecentLite）· `service/ai/openai.go`（finish_reason 兼容）· `service/chat_tools.go` + `chat.go`（命名 AIChatService/AIChatToolExecutor）
- 前端：`api/chat.ts`（SSE data.data 修复）· `hooks/useChat.ts`（历史消息还原 toolStates）· `views/chat/MessageBubble.tsx`（语法高亮 + 链接埋点）· `components/chat/ArticleCardBlock.tsx`（卡片埋点）· `views/dashboard/ai.tsx`（看板页面）· `views/article/read.tsx`（返回按钮兼容新标签）
**技术决策**:
- 命名规范：接口通用名，实现用 `{技术}{业务}{领域}{层}` 组合前缀（如 `CacheAIClickEventRepository`）
- 去重用 `uk_dedup`（user+article+conversation+source），`ON CONFLICT DO NOTHING`
- 看板缓存 10min TTL + jitter，写入后清缓存
- `ListRecentLite` 排除 tool_calls 字段优化 buildPrompt 性能
- 工具结果序列化存入 message.tool_calls，前端加载历史时还原卡片
**待办**: Phase 6 意图识别 + 路由分发（FAQ / RAG / Tool）
**会话**: 260410-chat-埋点看板

## [2026-04-06] Chat Function Calling — 工具调用 + 热门文章精确排行

**变更内容**: Chat 接入 OpenAI Function Calling，实现三个工具（文章搜索、热门推荐、我的收藏）；热门文章改为按互动加权分（read×1 + like×3 + collect×5）真实排行；前端渲染工具执行状态和文章卡片
**影响范围**: `service/ai/llm.go` · `service/ai/openai.go`（streaming tool_call 拼接）· `service/chat.go`（runStream 多轮工具循环）· `service/chat_tools.go`（新增 ToolExecutor）· `repository/dao/interaction.go`（ListHotBizIds / ListCollectedBizIds）· `domain/chat.go`（ArticleCard / ToolResultData）· `wire.go` · `hooks/useChat.ts` · `components/chat/ArticleCardBlock.tsx` · `views/chat/MessageBubble.tsx`
**技术决策**: 工具调用循环最多 5 轮防无限循环；executor 为 nil 时降级（兼容无工具场景）；热门排行加权而非最新，`stream_options include_usage=true` 修复 token 用量归零问题；system prompt 规则 8 强制每次重新调用动态数据工具
**待办**: Phase 6 意图识别 + 路由分发（FAQ / RAG / Tool）

## [2026-04-06] Chat RAG — 基于文章内容的知识问答

**变更内容**: 聊天接入 RAG，用户提问时自动检索平台文章，将相关内容注入 prompt，AI 基于文章回答并附带可点击链接
**影响范围**: `service/chat.go`（RAG 逻辑）· `wire.go`（DI 注入 ArticleSearchService）· `views/chat/MessageBubble.tsx`（链接新标签页打开）· `views/article/read.tsx`（返回按钮兼容新标签页）· `.gitignore`（排除 .next 构建产物）
**技术决策**: 复用已有 ArticleSearchService（hybrid BM25 + 向量），top-3 召回注入 system message；直接提供完整 Markdown 链接防止 LLM 编造 ID；检索失败静默降级不阻塞对话
**待办**: 阶段 B Function Calling（search_articles / list_favorites / hot_articles）
**会话**: 260406-chat-RAG

## [2026-04-05] Embedding 分包 + 分页状态保持

**变更内容**: ai/embedding 拆为独立子包，缓存移到 cache 层；文章列表分页同步 URL，编辑返回不丢页码
**影响范围**: `service/ai/embedding/`（新子包）· `cache/embedding.go`（缓存归位）· `views/article/list.tsx` · `views/article/edit.tsx`
**技术决策**: 子包按能力拆分（LLM vs Embedding），接口用完整名 `EmbeddingClient` 避免歧义；分页用 URL searchParams 而非 state，支持刷新保留

## [2026-04-05] 本地 Ollama Embedding + 收费模型降级

**变更内容**: 向量化优先走本地 Ollama bge-m3，失败自动降级到阿里百炼收费 API
**影响范围**: `ai/ollama_embedding.go` · `ai/failover_embedding.go` · `ioc/es.go` · `config/`
**技术决策**: 复用 `EmbeddingClient` 接口 + `FailoverEmbeddingClient` 顺序尝试，外层 `CachedEmbeddingClient` 不变
**会话**: 260405-embedding-ollama-failover

## [2026-04-05] 搜索优化 + 时间 int64 统一 + 性能修复

**变更内容**: 搜索功能全链路优化、时间字段统一为 int64 毫秒时间戳、多项性能和缓存修复
**影响范围**: DAO/domain/repository/service/web 全层 + 前端 types/views + ES mapping + SQL schema
**技术决策**:
- 时间用 int64 而非 time.Time：消除时区歧义，全链路零转换，前端 dayjs 格式化
- 搜索用 script_score 替代顶层 kNN：避免 kNN OR 合并返回全部文档
- Embedding 加 Redis 缓存（装饰器模式）：相同查询不重复调 API
- Wire/Mock 禁止手动改生成文件，统一用命令生成
**待办**: FindByBizIds 批量缓存、ES kNN+RRF 混合搜索（需 ES 8.8+）
**会话**: 250404-搜索优化-时间统一

## [2026-04-04] 安全修复：Chat 越权删除消息 + 越权取消生成

**变更内容**: 修复 3 处安全 / 架构问题
**影响范围**: `dao/chat_conversation.go` · `service/chat.go` · `service/chat_test.go`
**技术决策**:
1. `Delete` 事务先删 Conversation（uid 校验 + RowsAffected）再删 Messages，防越权删他人消息
2. `cancel` map 改用 `uid:convId` 复合 key，防任意用户 `/chat/stop` 取消他人生成
3. `isNotFound` 改用 `repository.ErrRecordNotFound`，service 层不再直接依赖 GORM
**会话**: 260404-chat-security-fix

## [2026-04-04] AI 客服多 LLM 故障转移 + 项目治理

**变更内容**: 新增 Kimi LLM 提供方，实现 FailoverClient（轮询）+ TimeoutFailoverClient（粘性超时切换）双策略故障转移；前端 SSE 超时断开 + 业务错误即断；consts 按领域拆分；CLAUDE.md 精简为导航+规则
**影响范围**: `service/ai/`（新增 4 文件）、`service/chat.go`（forwardStream 防泄漏重写）、`config/types.go`、`ioc/chat.go`、`consts/`（拆 3 文件）、`api/chat.ts`、`hooks/useChat.ts`、`CLAUDE.md` × 3
**技术决策**: 参考 SMS failover 拆为两种独立策略（单一职责），不合并；超时计入故障但不轮询（网络差时备用也超时）；业务错误不计入 cnt（切 provider 也没用）；forwardStream 所有 channel 写入改为 select ctx.Done 防 goroutine 泄漏
**待办**: 集成测试 SSE 用例需要有效 API Key 才能跑通
**会话**: 240404-全栈-AI客服故障转移

## 2026-03-29

### 读者端首页分页缓存 + 代码质量修复

**变更内容**:
- 读者端分页首页缓存（方案 C：只缓存第一页，TTL 3min）
- DAO 层 Insert/Update/Upsert 自动补充 abstract（`ensureAbstract`）
- 互动模块：logger 注入替代 `zap.L()`、事务包裹、错误码补充
- 前端 `Modal.confirm` → `modal.confirm`（antd 规范）
- SQL 补充 abstract 列和数据
- mock.mk 修复 article mock 生成路径

**影响范围**: `repository/cache/article.go`、`repository/article_reader.go`、`dao/article_author.go`、`dao/article_reader.go`、`dao/interaction.go`、`repository/interaction.go`、`web/interaction.go`、`webook-fe/views/article/list.tsx`、`scripts/webook.sql`

### 点赞/收藏功能（第2期互动模块）

**变更内容**: 新增文章互动功能，包含阅读量上报、点赞、收藏，前后端全栈实现
**影响范围**:
- 后端：domain/interaction.go（新），dao/interaction.go（新，`interaction`+`user_interaction` 两张表），repository/interaction.go（新），service/interaction.go（新），web/interaction.go（新），ioc/web.go，wire.go，wire_gen.go，init_table.go
- 前端：types/interaction.ts（新），api/interaction.ts（新），views/article/read.tsx（更新），types/index.ts（更新）

**新增接口**:
- `POST /interaction/read` — 阅读上报（公开，无需登录）
- `POST /interaction/get` — 获取聚合计数+用户状态（公开，登录后有 liked/collected 状态）
- `POST /interaction/like` — 点赞/取消点赞（需登录）
- `POST /interaction/collect` — 收藏/取消收藏（需登录）

**技术决策**:
- 聚合计数独立表 `interaction`（article_id unique），避免污染 published_article
- 用户操作记录独立表 `user_interaction`（user_id+article_id 联合唯一索引）
- 计数用 `GREATEST(0, count + delta)` 防止下界越零
- 前端乐观更新：点赞/收藏立即更新本地状态，失败时显示错误提示
- 阅读上报在文章内容加载完成后触发，失败静默处理不影响阅读体验
- `/interaction/read` 和 `/interaction/get` 加入 JWT 白名单（允许匿名访问）

**会话**: 260329-like-collect

---

## 2026-03-26

### 文章发布/撤回功能

**变更内容**: 实现文章发布（制作库+线上库双表事务）和撤回（幂等，只对已发布文章改状态）功能
**影响范围**: domain/article.go, dao/article_author.go, dao/article_reader.go, repository/article_author.go, service/article.go, web/article.go, wire.go, integration/article_test.go
**技术决策**:
- 制作库/线上库用 author/reader 命名，发布事务合并到 ArticleAuthorDAO（作者的操作）
- 撤回采用幂等设计：已发布→Private，草稿/已撤回→不改状态，线上库 DELETE 幂等
- DAO 层不依赖 domain 包，状态常量只在 domain 定义一份，通过 Repository 传参
- ArticleReaderDAO 暂时只保留 PublishedArticle 模型，接口和实现待未来读取功能时添加
**待办**: Publish 接口缺少"发布不存在文章"测试用例
**会话**: 260326-article-publish

---

## 2026-03-25

### 学习笔记：第01-03周

生成专业学习笔记，覆盖 Go 基本语法、Gin/GORM 入门、Session/JWT 认证、Kubernetes 部署入门。按知识体系重新组织，补充底层原理、最佳实践、对比表格和面试要点。

**文件:** `notes/_sections/sec_01_03.md`（2400+ 行）

---

## 2026-03-08

### 微信 OAuth2 登录

微信扫码登录完整流程：AuthURL 生成授权链接 → 回调获取 code → 换取 access_token + openid → 查找/创建用户 → 签发双 Token

**文件:** `internal/web/wechat.go` / `internal/service/oauth2/wechat/service.go` / `internal/domain/wechat.go` / `ioc/wechat.go`

### JWT 双 Token 认证（长短 Token）

access token 30min + refresh token 7天，refresh 时 ssid 不变只重签 access token，logout 时 Redis 存 ssid 使会话失效

**文件:** `internal/web/jwt/handler.go` / `internal/web/jwt/types.go` / `internal/web/user.go`

### SMS 装饰器：权限控制 / 限流 / 故障转移

SMS 服务支持多种装饰器组合：JWT 权限校验(`sms/auth/`) / Redis 滑动窗口限流(`sms/ratelimit/`) / 轮询故障转移(`sms/failover/`) / 连续超时切换(`sms/failover/timeout_failover.go`)

### 集成测试框架

Wire 独立 setup + MySQL/Redis 容器化测试环境，覆盖用户注册/登录/微信登录等核心流程

**文件:** `internal/integration/` / `internal/integration/setup/`

## 2026-03-15

### Docker Compose 本地开发环境

从 K8s deployment 文件中提取 MySQL 8.0、Redis 7.0、etcd 3.5.17 三个基础设施服务，生成 `docker-compose.yaml`，方便本地 `docker compose up -d` 一键启动开发依赖

**文件:** `docker-compose.yaml`

### Logger 中间件

请求日志中间件，Builder 模式构建，记录 Path/Query/Method/ClientIP/UserAgent/ReqBody/ResBody/Status/Duration，ReqBody 和 ResBody 截断 2048 字节

**文件:** `internal/web/middleware/logger.go`

### Logger 配置模块

基于 viper 配置文件路径判断环境（dev → DevelopmentConfig，k8s → ProductionConfig），通过 `LoggerConfig` 中间结构体将 yaml 配置逐字段覆盖到 `zap.Config`，解决 `viper.UnmarshalKey` 无法反序列化 `AtomicLevel`/`EncoderConfig` 的问题

**文件:** `ioc/logger.go` / `config/dev.yaml` / `config/k8s.yaml`

### Docker Compose webook 服务

webook 服务加入 docker-compose，传递 `APP_ENV=config/k8s.yaml` 环境变量，Dockerfile 增加 COPY config 目录

**文件:** `docker-compose.yaml` / `Dockerfile`

### 全局日志替换

项目中所有 `log.Printf`/`log.Println`/`fmt.Printf` 调试打印替换为注入式 `logger.LoggerX`，通过构造函数注入而非全局变量，涉及 user handler、failover SMS、memory SMS、ratelimit builder、login middleware。补充 `logger.Int32`/`Uint64`/`Strings` 辅助函数

**文件:** `internal/web/user.go` / `internal/service/sms/failover/` / `internal/service/sms/memory/` / `pkg/ginx/middleware/ratelimit/builder.go` / `internal/web/middleware/login.go` / `pkg/logger/fields.go`

### 集成测试 viper 初始化

添加 `TestMain` 入口加载 `config/dev.yaml`，修复集成测试无法连接数据库的问题

**文件:** `internal/integration/init_test.go` / `internal/integration/setup/wire.go`

## 2026-03-17

### 时间类型改造：int64 → time.Time

DAO 层和 Domain 层的 `Birthday`/`CreatedAt`/`UpdatedAt` 从 `int64` 毫秒时间戳改为 `time.Time`，MySQL 存储从 `BIGINT` 改为 `DATETIME`，和 MongoDB BSON Date 行为一致

**策略:** 全链路 UTC 存储 + carbon 东八区查询

**配置:**
- DSN: `parseTime=true&loc=UTC` — MySQL 驱动按 UTC 解析 datetime
- `ioc/time.go`: `carbon.SetTimezone("Asia/Shanghai")` — carbon 全局东八区
- `main.go`: 启动时调用 `ioc.InitTimezone()`

**时区处理规则:**
- 存储: 前端传毫秒时间戳 → `time.UnixMilli(ts).UTC()` → 存入 MySQL datetime（UTC）
- 查询: `carbon.Now().StartOfDay().StdTime()` 自动按东八区算一天的开始/结束
- 展示: 前端 `dayjs()` 自动将 UTC 转本地时区显示

**踩坑:**
1. `time.Time` JSON 反序列化只认 RFC3339，前端传时间戳必须用 `int64` 接收
2. `dayjs(value, 'YYYY-MM-DD')` 带格式参数会丢时区信息导致日期少一天，应用 `dayjs(value)`
3. carbon 默认用 UTC，必须 `SetTimezone` 后 `StartOfDay()` 才是东八区零点

**文件:** `ioc/time.go` / `ioc/db.go` / `config/dev.yaml` / `config/prd.yaml` / `internal/repository/dao/user.go` / `internal/domain/user.go` / `internal/domain/article.go` / `internal/repository/user.go` / `internal/web/user.go`
