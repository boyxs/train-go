# L1 部署 - 一份 compose + env 切换

> 项目**部署唯一真相源**。根目录的 docker-compose.yaml / nginx/ / prometheus/ / grafana/ / otel-collector/ 全部合并到这里。K8s 将来并列开 `k8s/` 或 `helm/` 目录。

## 速查目录

- [快速开始](#快速开始)
- [CI/CD 流程](#cicd-流程按-env-讲清楚)
- [日常操作](#日常操作)
- [环境配置](#环境配置env变量清单)
- [调试与排障](#调试与排障查问题解问题)
- [文件布局](#文件布局)
- [L2 K8s 迁移对照](#l2-k8s-迁移对照)

---

## 快速开始

### 四种启动模式

| 命令 | 镜像来源 | 业务中间件端口 | 监控端口 | 适合场景 |
|------|---------|--------------|--------|---------|
| `./deploy.sh local` | 本地 build | **全暴露**（3306/6379/9092/9200 等） | 80/3001/9090/9411 | Windows 开发，`go run` / DBeaver 连中间件 |
| `./deploy.sh dev` | ghcr (`main-latest`) | 不暴露 | 80/3001/9090/9411 | 团队 dev 服务器 |
| `./deploy.sh staging` | 同上 | 不暴露 | **81/3011/9190/9412** | 预发布 |
| `./deploy.sh prod` | ghcr (`1.2.0`，语义化版本) | 不暴露 | **82/3021/9290/9413** | 生产 |

> 💡 `up` 只在镜像**缺失时**拉取（compose `pull_policy: missing`）。**同 tag 远端更新**需显式 `./deploy.sh <env> pull` 后再 up，否则继续用本地缓存。

**关键约束**：
- 同时只能跑一个（container_name 全 docker 唯一）
- 切 env 自动 stop 旧的
- volume 按 project 隔离（`webook-<env>_mysql-data`），切回数据还在
- 想访问 staging 时一定走 `:81`；访问 prod 走 `:82`

### 首次部署

**本地开发**：
```bash
cd deploy
./deploy.sh local                # build 代码 + 起全套
# 访问：http://localhost  (API: /api)
```

**服务器部署**：
```bash
# 1. 传文件
scp -r deploy/ user@server:~/webook-deploy/

# 2. 服务器配置
ssh user@server
cd ~/webook-deploy
chmod +x deploy.sh

# 3. 登录 GHCR（镜像 private 时）
echo "ghp_xxx" | docker login ghcr.io -u YOUR-GH-USER --password-stdin

# 4. 改 .env.<env>（至少 GH_USER；prod 改强密码）
vim .env.dev

# 5. 起
./deploy.sh dev
```

### ghcr 镜像源切换

默认 `GHCR_REGISTRY=ghcr.io`（官方原站）。国内服务器拉取慢时，按需换加速域。

> ⚠️ **前提**：GHCR package 已在 GitHub → Packages → visibility 改为 **public**。私有 package 请保持默认 `ghcr.io` + `docker login`，第三方加速域（nju 等公共 pull-through cache）对私有包一律返回 404。

**改 `.env.<env>` 永久生效**：
```bash
# vim .env.prod
GHCR_REGISTRY=<你验证过的 ghcr 加速域>      # 例：ghcr.nju.edu.cn
```

**`--ghcr` flag 单次覆盖（不改文件）**：
```bash
./deploy.sh prod --ghcr <mirror>
./deploy.sh prod pull --ghcr <mirror>      # 只拉不起
```

注意事项：
- `--ghcr` 值**不带尾斜杠**；脚本自动规范化
- 加速域未同步到最新 tag 时会 404——用 `--ghcr ghcr.io` 回退官方
- **只影响自家 ghcr 镜像**（webook-core / webook-chat / webook-fe）；Docker Hub 镜像走 daemon `registry-mirrors`，和本 flag 无关

#### 方案：阿里云 ACR 做 ghcr 反代（推荐，支持私有包）

公共 ghcr 加速域（nju 等）拉不到私有 package；阿里云 ACR 的「镜像同步」能把 ghcr.io 私有镜像拉进你自己的阿里命名空间，prod 再从阿里拉，速度快且私有。

**1. 开通阿里云 ACR 个人版**

[容器镜像服务 ACR](https://cr.console.aliyun.com) → 开通个人实例（免费）→ 选地域（推荐 `cn-hangzhou` 或离部署机最近的）→ 创建命名空间（下文记为 `<ACR_NS>`，公开/私有都行）。

**2. 绑定 ghcr 为「访问凭证来源」**

ACR 控制台 → 实例列表 → 选中实例 → 「镜像仓库 → 仓库同步 → 访问凭证」→ 添加 `ghcr.io` 凭证（用户名：GitHub 用户名；密码：GitHub PAT，勾 `read:packages` 权限）。

**3. 创建同步规则**

ACR「仓库同步 → 同步规则 → 新建」（**三个仓库各加一条**）：
- 源仓库：`ghcr.io/<你的 GH 用户名>/webook-core`
- 目标仓库：`<你的 ACR 实例>/<ACR_NS>/webook-core`
- tag 过滤：`1.*` 或 `.*`
- 触发：手动 / 定时 / 源有变更就触发（推荐最后一种）
- `webook-chat` / `webook-fe` 各重复一次

**4. 改 `.env.<env>`**

```bash
# .env.prod（只有这两个变量要改）
GHCR_REGISTRY=registry.cn-hangzhou.aliyuncs.com
GH_USER=<ACR_NS>                                # 改成阿里云命名空间名
```

compose 里 `${GHCR_REGISTRY}/${GH_USER}/webook-core:${CORE_IMAGE_TAG}` 渲染成 `registry.cn-hangzhou.aliyuncs.com/<ACR_NS>/webook-core:1.2.0`（chat / fe 同理）。

**5. 服务器 docker login 阿里云**

```bash
# 用 --password-stdin 避免密码进 shell history / ps 输出
echo '<ACR 访问凭证>' | docker login registry.cn-hangzhou.aliyuncs.com -u <阿里账号> --password-stdin
# ACR 访问凭证：控制台「实例 → 访问凭证」页生成（和阿里主账密码分开）
```

注意事项：
- **同步延迟**：源有变更触发的规则通常 1-3 min 内完成；定时规则看配置。发版后 prod 拉不到新 tag 先查同步状态
- **私有→私有**：阿里 ACR 命名空间设私有即可，对外不可拉；`docker login` 凭证只发给 prod
- **配额**：个人版 ACR 有免费额度（具体配额以[控制台](https://cr.console.aliyun.com)为准）；企业版按量付费
- 想省事可以用「主账号 → AccessKey」做 docker login，但权限过大；推荐走 ACR 「访问凭证」只给 pull 权限

---

## CI/CD 流程（按 env 讲清楚）

### 整体架构

```
开发者电脑
  ↓ git push
GitHub
  ↓ 触发 workflow
GitHub Actions
  ├── lint-test (go vet + goimports + go test)
  └── build-push (go build + docker build + push ghcr)
       ↓
ghcr.io/<user>/webook-core:<tag> + webook-chat:<tag> + webook-fe:<tag>
       ↓ ./deploy.sh <env> 去拉
部署机（CentOS/Linux）
  ↓
webook-<env> project（业务 + 中间件 + 监控全套容器）
```

### CI：分支 / tag 推什么镜像

CI 配置三份独立 workflow，互斥 `paths-ignore`：
- `.github/workflows/webook-core-ci.yml` — 监听 `webook/internal/** webook/pkg/** webook/api/**`
- `.github/workflows/webook-chat-ci.yml` — 监听 `webook/chat/** webook/pkg/** webook/api/**`
- `.github/workflows/webook-fe-ci.yml` — 监听 `webook-fe/**`

| 事件 | 触发的 workflow | 镜像 tag |
|------|---------------|---------|
| `git push feature/<anything>` | 改了哪个服务就触发哪个 | `feature-<sha>` |
| `git push main` | 改了哪个服务就触发哪个 | `main-latest` + `main-<sha>` |
| `git tag webook-core-v1.2.0 && git push --tags` | core CI | `webook-core:1.2.0` |
| `git tag webook-chat-v1.2.0 && git push --tags` | chat CI | `webook-chat:1.2.0` |
| `git tag webook-fe-v1.2.0 && git push --tags` | fe CI | `webook-fe:1.2.0` |

CI 做了哪些加速（`perf(ci): 4 项加速改造`）：
- `concurrency.cancel-in-progress: true` — 快速连推取消旧 run
- `feature/**` 分支跳 `-race`（单测快 2-5x）
- `paths-ignore: deploy/ docs/ sandbox/ prd/ **.md` — 改这些不触发后端 CI
- goimports 条件安装 + pin 版本走 setup-go cache
- `GOFLAGS: -mod=readonly` — 禁 CI 改 go.sum 防依赖漂移

### CD：各 env 的部署流程

#### 📘 Local（本地开发，不走 CI）

```bash
# Windows / 本地 docker desktop
cd deploy
./deploy.sh local    # 自动 build 本地代码 + up
```

- 镜像：**本地 build**（不拉 ghcr）
- `docker-compose.local.yaml` override 暴露全部中间件端口给 DBeaver / IDE
- 改 Go 代码 → 再跑 `./deploy.sh local build && ./deploy.sh local` 重 build
- 前端改代码同理，Next.js build 较慢（2-3min）

#### 📗 Dev（团队共享 dev 服务器）

```bash
# === 开发者侧 ===
git checkout -b feature/xxx
# 改代码...
git push origin feature/xxx
# CI 按改动路径自动推 ghcr.io/<user>/webook-{core,chat,fe}:feature-<sha>（只触发受影响的服务）
# 想合就开 PR → merge 到 main
# main push 触发 CI → 推 main-latest + main-<sha>

# === 服务器侧（dev 机）===
ssh dev-server
cd ~/webook-deploy
./deploy.sh dev
# 从 ghcr 拉 main-latest，起 webook-dev project
# 访问：http://<dev-server>/
```

- `.env.dev` 的 `CORE_IMAGE_TAG` / `CHAT_IMAGE_TAG` / `FE_IMAGE_TAG` 默认 `main-latest` → 每次 `./deploy.sh dev` 都拉最新 main
- 想回退到某个特定 sha：改 `.env.dev` 的对应 `*_IMAGE_TAG`：
  ```bash
  sed -i "s|^CORE_IMAGE_TAG=.*|CORE_IMAGE_TAG=main-a1b2c3d|" .env.dev
  ./deploy.sh dev pull && ./deploy.sh dev
  ```
- CI 在 main 走 `-race` 测试，质量闸门保留

#### 📒 Staging（预发布）

```bash
# 和 dev 拉同一个 main-latest，只是资源规格贴近 prod + 端口错开
ssh staging-server
cd ~/webook-deploy
./deploy.sh staging
# 访问：http://<staging-server>:81/
```

- `.env.staging` 的 `CORE_IMAGE_TAG` / `CHAT_IMAGE_TAG` / `FE_IMAGE_TAG` 都是 `main-latest`（和 dev 同源）
- 用途：上 prod 前在 staging 跑一轮 smoke test / E2E
- 如果用独立 staging 机，和 dev 完全隔离；如果和 dev 同机会互相 stop（container_name 冲突）

#### 📕 Prod（生产）

```bash
# === 发版人侧 ===
# main 充分测试后按需打 tag（每个服务独立打，不必三件套都打）
git tag webook-core-v1.2.0
git tag webook-chat-v1.2.0
git tag webook-fe-v1.2.0
git push --tags
# 三个 CI 各自触发，分别推 ghcr.io/<user>/webook-{core,chat,fe}:1.2.0

# === 服务器侧（prod 机）===
ssh prod-server
cd ~/webook-deploy

# 改 .env.prod 的 IMAGE_TAG（发版 / 回滚都走这个）
sed -i "s|^CORE_IMAGE_TAG=.*|CORE_IMAGE_TAG=1.2.0|" .env.prod
sed -i "s|^CHAT_IMAGE_TAG=.*|CHAT_IMAGE_TAG=1.2.0|" .env.prod
sed -i "s|^FE_IMAGE_TAG=.*|FE_IMAGE_TAG=1.2.0|" .env.prod

./deploy.sh prod pull   # 先拉新镜像（失败不覆盖现有容器）
./deploy.sh prod        # up 新版本
# 访问：http://<prod-server>:82/
```

- **prod 建议只填语义化版本**（`x.y.z`）到 `.env.prod` 的三个 `*_IMAGE_TAG`，不要填 `main-latest`（防止手滑把未验证版本发到生产）
- **回滚**：改 `.env.prod` 的 `CORE_IMAGE_TAG` / `CHAT_IMAGE_TAG` / `FE_IMAGE_TAG` 回旧版本（例 `1.1.0`）然后 `./deploy.sh prod pull && ./deploy.sh prod`
- GHCR 上的 tag 只要没手动删就永久保留，回滚无时效限制

### 发版 checklist（prod 上线）

```bash
# 1. 代码合到 main 且 CI 绿灯
# 2. 在 dev / staging 跑过 smoke test
# 3. 打 tag（按需，每服务独立）
git tag webook-core-v1.2.0
git tag webook-chat-v1.2.0
git tag webook-fe-v1.2.0
git push --tags

# 4. 等 CI 绿灯（https://github.com/<user>/<repo>/actions）
#    三个 workflow 各自把 1.2.0 镜像推到 GHCR

# 5. 同步 .env.prod.example（git tracked，CLAUDE.md 发版流程要求）
#    实际 .env.prod 由部署者按 example 同步（gitignored 文件）

# 6. SSH 到 prod 服务器改 .env.prod 三个 *_IMAGE_TAG 再部署
ssh prod-server
cd ~/webook-deploy
sed -i "s|^CORE_IMAGE_TAG=.*|CORE_IMAGE_TAG=1.2.0|" .env.prod
sed -i "s|^CHAT_IMAGE_TAG=.*|CHAT_IMAGE_TAG=1.2.0|" .env.prod
sed -i "s|^FE_IMAGE_TAG=.*|FE_IMAGE_TAG=1.2.0|" .env.prod
./deploy.sh prod pull    # 先拉镜像，确保都能拉下来
./deploy.sh prod         # up 新版本

# 7. 验证
./deploy.sh prod status
curl http://<prod-server>:82/healthz

# 8. 出问题立即回滚（把对应 *_IMAGE_TAG 改回旧版本重来；按需只回滚某个服务）
sed -i "s|^CORE_IMAGE_TAG=.*|CORE_IMAGE_TAG=1.1.0|" .env.prod
sed -i "s|^CHAT_IMAGE_TAG=.*|CHAT_IMAGE_TAG=1.1.0|" .env.prod
sed -i "s|^FE_IMAGE_TAG=.*|FE_IMAGE_TAG=1.1.0|" .env.prod
./deploy.sh prod pull && ./deploy.sh prod
```

### 三服务独立发版

CI 通过 `paths-ignore` 严格分离三个服务：core / chat / fe 改动只触发自家 workflow。所以：

- 只改 core 业务：`git push main` → 只触发 core CI → 只更新 `webook-core:main-latest`
- 只改 chat 业务：同上触发 chat
- 只改前端：同上触发 fe
- 紧急修某一服务：独立打 tag（如 `webook-chat-v1.2.1`）独立发版

**共享代码改动**（`webook/pkg/**` 或 `webook/api/**`）会**同时触发** core + chat（两个 CI 都监听这些路径）。

### CI 的镜像哪里看

```
https://github.com/<user>?tab=packages
```

看 `webook-core` / `webook-chat` / `webook-fe` 三个 package，各自的 Versions 标签列出所有 tag。

### 触发 CI 的排错

CI 没跑起来？按顺序检查：
1. **最常见：commit 被 `paths-ignore` 跳过** — core CI 忽略 `webook-fe/ webook/chat/ deploy/ docs/ sandbox/ prd/ **.md`；chat CI 忽略 `webook-fe/ deploy/ docs/ sandbox/ prd/ **.md`；fe CI 忽略 `webook/ deploy/ docs/ sandbox/ prd/ **.md`。**这是故意的**，不是 bug
2. **feature 分支名不是 `feature/xxx`**（注意是斜杠不是下划线）→ 不在 `branches: ['feature/**']` 列表，不触发
3. **workflow 文件有语法错误** → Actions 页面会报 `Startup failure`
4. **GITHUB_TOKEN 权限不够** → 看 Settings → Actions → Workflow permissions 是否 `Read and write`
5. **push 被 concurrency cancel** — 快速连推时旧 run 会被新 run 取消，Actions 页面显示 `Cancelled` 是正常行为

---

## 日常操作

### 基本命令

```bash
./deploy.sh <env> [up] [service...]     # up（自动 stop 其他 env；指定服务含各自依赖链，可传多个）
./deploy.sh <env> down                  # 停整个 env（volume 保留；不接服务名，按服务用 stop/rm）
./deploy.sh <env> stop [service...]     # 停全部或指定服务（容器保留，up 即恢复）
./deploy.sh <env> rm <service...>       # 停并移除指定服务容器（volume 保留，up 会重建）
./deploy.sh <env> nuke                  # 停 + 清 volume（prod 需二次确认）
./deploy.sh <env> logs [service...]     # 跟日志（默认 webook-core，可多个混合跟踪）
./deploy.sh <env> status [service...]   # docker compose ps
./deploy.sh <env> pull [service...]     # 拉镜像
./deploy.sh <env> build [service...]    # 构建镜像（local 用）
./deploy.sh <env> restart <service...>  # 重启指定服务
./deploy.sh list                        # 所有 env 残留总览
```

单服务/多服务示例：

```bash
./deploy.sh local up webook-core webook-chat   # 只构建+启动这两个（mysql 等依赖自动带起）
./deploy.sh dev stop webook-worker             # 单停调度器
./deploy.sh dev rm webook-worker webook-migrator
```

### 启用 LLM（ollama，默认关）

```bash
COMPOSE_PROFILES=llm ./deploy.sh <env>
```

**角色定位**：当前 Ollama 只服务 **embedding**（文章向量化 → ES kNN 搜索）。core 配置 `ollama.model: bge-m3`（`webook/internal/config/*.yaml`），嵌入链路 Ollama 优先 → dashscope（阿里，计费）兜底；**chat 对话走外部 API，不经 Ollama**。

**首次启用必须手动拉模型**（模型不随镜像分发，落 `ollama-data` volume，重建容器不用重拉）：

```bash
docker exec webook-ollama ollama pull bge-m3    # 1.2GB
docker exec webook-ollama ollama list           # 确认模型已就位
# local 模式宿主可直连验证；server 模式端口未暴露且容器内无 curl，用上面 list + 业务日志确认
curl http://localhost:11434/api/embeddings -d '{"model":"bge-m3","prompt":"测试"}'
```

**嵌入模型选型**——硬约束：**必须 1024 维**对齐 ES `article_v1` mapping（`dims: 1024`），换维度 = 改 mapping + 重建索引：

| 模型 | 大小 | 维度 | 场景/特点 | 本项目价值 |
|------|------|------|----------|-----------|
| **bge-m3** ⭐ | 1.2GB | 1024 | 中文/多语言首选，支持长文本，RAG 常用 | 唯一零改动可用（配置已指定）；替代 dashscope 计费 API，断网可用 |
| snowflake-arctic-embed2 | 1.2GB | 1024 | 多语言检索（arctic 二代） | 质量相当但需改配置，无增量收益 |
| snowflake-arctic-embed:335m | ~670MB | 1024 | 英文检索强（一代 `l` 档；另有 xs/s/m 档，384/768 维） | 维度可用但中文弱 |
| mxbai-embed-large | 670MB | 1024 | 英文检索 | 省 500MB 内存，中文明显弱 |
| nomic-embed-text | ~274MB | 768 | 英文性价比最优，效果超 OpenAI ada-002，8192 长上下文 | 维度不匹配¹ |
| shaw/dmeta-embedding-zh | ~400MB | 768 | 社区中文模型，中文小体积之选 | 维度不匹配¹ |
| all-minilm | ~46MB | 384 | 最小最快，英文场景够用 | 维度不匹配¹ |

> ¹ 非 1024 维 = 改 ES mapping `dims` + 重建 `article_v1` 索引 + 全量重嵌入，除非有硬需求否则不划算。

**对话模型**（当前代码未接入，仅供将来给 chat 加本地 provider 参考；约束：`OLLAMA_MEM=2048m` + CPU 推理 ≈5-15 tokens/s）：

| 模型 | 大小 | 场景 | 价值 |
|------|------|------|------|
| qwen2.5:1.5b | ~1.0GB | 中文对话/摘要 | 2G 限额内中文性价比最高 |
| deepseek-r1:1.5b | ~1.1GB | 推理/思维链 | 学习 R1 蒸馏行为，实用弱 |
| llama3.2:1b / gemma3:1b | ~0.8GB | 英文轻对话 | 中文弱 |
| gemma2:2b | ~1.6GB | 通用小模型 | 质量最好但 +KV cache 贴 2048m 限额，易 OOM |

> 8G CPU-only 机器上：对话模型 = 学习价值 > 实用价值；嵌入是真实用（bge-m3 CPU 单条毫秒级）。3B 及以上超限额不列。

**两个坑**：

- `.env.dev` 的 `OLLAMA_MEM=1024m` **装不下 bge-m3**（权重 1.2GB），dev 启用 llm profile 前先调 ≥2048m
- **冷启动超时属预期**：模型加载 5-15s > core `ollama.timeout`(5s)，且闲置 5min 自动卸载——冷请求 failover 到 dashscope，后续毫秒级。想常驻内存打开 compose 里注释的 `OLLAMA_KEEP_ALIVE: "-1"`（代价：常驻 1.2GB，小内存机自行权衡）

### 发版

```bash
# 本地（按需打 tag，每个服务独立）
git tag webook-core-v1.2.1
git tag webook-chat-v1.2.1
git tag webook-fe-v1.2.1
git push --tags
# 三个 CI 各自触发，分别推 ghcr.io/<user>/webook-{core,chat,fe}:1.2.1

# 服务器（等 CI 绿灯）
ssh prod-server
cd ~/webook-deploy
sed -i "s|^CORE_IMAGE_TAG=.*|CORE_IMAGE_TAG=1.2.1|" .env.prod
sed -i "s|^CHAT_IMAGE_TAG=.*|CHAT_IMAGE_TAG=1.2.1|" .env.prod
sed -i "s|^FE_IMAGE_TAG=.*|FE_IMAGE_TAG=1.2.1|" .env.prod
./deploy.sh prod pull && ./deploy.sh prod
```

详细发版 checklist + 回滚方法见 [CD 流程 - Prod 章节](#-prod生产)。

---

## 环境配置（env 变量清单）

全部 env 变量都通过 `compose` 里的 `${VAR:-default}` 带默认值。改 `.env.<env>` 覆盖。

| 类别 | 变量 |
|------|------|
| 镜像 | `GH_USER`、`CORE_IMAGE_TAG`、`CHAT_IMAGE_TAG`、`FE_IMAGE_TAG`、`GHCR_REGISTRY`（默认 `ghcr.io`；私有 package 必须走官方源 + docker login） |
| webook 配置 | `CORE_APP_ENV`、`CHAT_APP_ENV`（各服务选哪份 yaml）、`DEPLOY_ENV` |
| 凭证 | `MYSQL_PASS` / `REDIS_PASS`（**必须和 `webook/config/<env>.yaml` 里 `mysql.dsn` / `redis.password` 一致**） |
| Kafka EXTERNAL | `KAFKA_EXTERNAL_HOST`（主机访问 kafka 的 advertised 地址，local=`localhost`，server=服务器 IP）、`KAFKA_EXTERNAL_PORT` |
| etcd | `ETCD_ALLOW_NONE`（yes=无认证，no=带 `ETCD_PASS`）、`ETCD_PASS`、`ETCD_MEM` |
| 业务/中间件内存 | `MYSQL_MEM`、`REDIS_MEM`、`KAFKA_MEM`、`KAFKA_HEAP`、`ES_MEM`、`ES_HEAP`、`FE_MEM`、`OLLAMA_MEM` |
| 监控栈内存 | `PROMETHEUS_MEM`、`GRAFANA_MEM`、`ZIPKIN_MEM`、`ZIPKIN_HEAP`、`OTEL_MEM`、`NGINX_MEM` |
| Exporter 内存 | `MYSQLD_EXPORTER_MEM`、`REDIS_EXPORTER_MEM`、`KAFKA_EXPORTER_MEM`、`NODE_EXPORTER_MEM` |
| 宿主端口（业务监控） | `NGINX_PORT`、`PROMETHEUS_PORT`、`GRAFANA_PORT`、`ZIPKIN_PORT`（四份 env 错开） |
| 宿主端口（local 专有） | `WEBOOK_HOST_PORT`、`MYSQL_HOST_PORT`、`REDIS_HOST_PORT`、`ETCD_HOST_PORT`、`KAFKA_HOST_PORT`、`KAFKA_EXTERNAL_HOST_PORT`、`ES_HOST_PORT`、`OLLAMA_HOST_PORT`、`OTEL_HOST_PORT` |
| Grafana | `GRAFANA_USER`、`GRAFANA_PASS`、`GRAFANA_SMTP_*`、`ALERT_EMAIL_TO` |

---

## webook 应用配置 yaml 对应关系

每个服务镜像各自打包 4 份 yaml（互不影响）：
- core：`webook/internal/config/{local,dev,staging,prod}.yaml`
- chat：`webook/chat/config/{local,dev,staging,prod}.yaml`

`.env.<env>` 的 `CORE_APP_ENV` / `CHAT_APP_ENV` 分别指定加载哪份。

| .env.<env> | CORE_APP_ENV | CHAT_APP_ENV | 用途 |
|-----------|--------------|--------------|------|
| `.env.local` | `config/local.yaml` | `config/local.yaml` | 本地 Docker 部署（host 端口暴露） |
| `.env.dev` | `config/dev.yaml` | `config/dev.yaml` | 团队 dev 服务器 |
| `.env.staging` | `config/staging.yaml` | `config/staging.yaml` | 预发 |
| `.env.prod` | `config/prod.yaml` | `config/prod.yaml` | 生产 |

Windows 上 `go run` 时设环境变量 `APP_ENV=config/local.yaml`（里面是 localhost 地址），**不走 docker 部署**——IDE 直连宿主端口。

---

## 调试与排障（查问题解问题）

### 🔧 从外部连容器内中间件（DBeaver / RedisInsight 等图形工具）

服务器部署模式（dev/staging/prod）下中间件**不暴露宿主端口**。要从本地工具连服务器数据库，用 **socat 桥**临时代理：

```bash
# 服务器上起桥（mysql 为例，--network 跟着当前 env 走，dev=webook-dev_default）
ssh root@server
docker run -d --name mysql-bridge --rm \
  --network webook-dev_default \
  -p 3306:3306 \
  alpine/socat TCP-LISTEN:3306,fork,reuseaddr TCP:webook-mysql:3306

# 本地 DBeaver 连 server-ip:3306，用户 root，密码见 .env.<env> 的 MYSQL_PASS

# 用完停桥
docker stop mysql-bridge
```

**其他中间件**（改容器名和端口）：

| 目标 | network | 目标容器 | 端口 |
|------|---------|--------|------|
| MySQL | `webook-<env>_default` | `webook-mysql` | 3306 |
| Redis | 同 | `webook-redis` | 6379 |
| Kafka | 同 | `webook-kafka` | 9092（PLAINTEXT） |
| ES | 同 | `webook-es` | 9200 |
| etcd | 同 | `webook-etcd` | 2379 |
| Ollama | 同 | `webook-ollama` | 11434 |

**优点**：不改 compose、不动数据、用完即停、不影响生产。

### 🔧 直接在容器里跑命令（不用本地工具）

```bash
# 导入 SQL 文件
cat data.sql | docker exec -i webook-mysql mysql -uroot -p13520 webook

# 交互式 SQL
docker exec -it webook-mysql mysql -uroot -p13520 webook

# 导入（从宿主文件）
scp data.sql root@server:/tmp/
ssh root@server 'docker exec -i webook-mysql mysql -uroot -p13520 webook < /tmp/data.sql'

# Redis
docker exec -it webook-redis redis-cli -a 13520

# Kafka topics
docker exec webook-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list

# ES
docker exec webook-es curl -s localhost:9200/_cat/indices
```

### 🔧 Windows 本地 `go run` 连容器内中间件

`./deploy.sh local` 模式下，docker-compose.local.yaml override 暴露全部中间件端口到宿主：

```
MySQL  :3306
Redis  :6379
Kafka  :9092（PLAINTEXT）+ 9094（EXTERNAL，go run 要用这个）
ES     :9200
etcd   :2379
```

Windows 上 `APP_ENV=config/local.yaml go run main.go`，local.yaml 里地址写 `localhost:*`，直接连。

---

## 常见问题排查

### 问 1：nginx 返回 504 Gateway Timeout

**症状**：POST 或 GET `/api/...` 60 秒后返回 504。

**根因链路**：webook-core 容器在 **restart loop**（不断 panic），nginx 转发到 webook 连接 hang。

**查**：
```bash
docker ps --filter "name=webook-core" --format "{{.Status}}"
# 如果显示 "Restarting (2) XX seconds ago" → 确认是 restart loop
docker logs webook-core --tail 30
# 找 panic 原因：常见 "failed to connect database"
```

**最常见原因**：
- `webook/internal/config/<env>.yaml` 或 `webook/chat/config/<env>.yaml` 的 `mysql.dsn` 密码和 `.env.<env>` 的 `MYSQL_PASS` 不一致
- → 改 yaml 或改 env 让两者一致，然后 `./deploy.sh <env> build && ./deploy.sh <env>`

### 问 2：docker pull 报 manifest unknown

**症状**：`./deploy.sh dev` 时某个镜像报 `manifest unknown`。

**排查**：
```bash
# 1. GHCR 上真有这个 tag 吗？
# 打开 https://github.com/YOUR_USER?tab=packages 看

# 2. 本地 docker 能直接 pull 吗？
docker pull ghcr.io/YOUR_USER/webook-core:main-latest
docker pull ghcr.io/YOUR_USER/webook-chat:main-latest
docker pull ghcr.io/YOUR_USER/webook-fe:main-latest
```

**常见原因**：
- `.env.<env>` 里 `*_IMAGE_TAG` 被 deploy.sh 意外写坏（比如 `down` 当 tag）→ 看 `grep IMAGE_TAG .env.dev`
- GHCR package 被删了或没推过 → 重新 push main 触发 CI 重建

### 问 3：容器频繁 Restarting + OOM

**症状**：某容器 Status 一直 Restarting，`docker inspect ... OOMKilled` 不一定为 true（cgroup OOM 不设这个标志），exit code 137。

**已知 OOM 阈值**（实机踩坑）：
- **MySQL 8.0 InnoDB init**：`MYSQL_MEM < 400m` 会被 cgroup OOM kill → 至少 **512m**
- **Kafka LogCleaner**：`KAFKA_HEAP < -Xmx384m` 会 Java OOM → 至少 **-Xmx512m**

**查**：
```bash
docker inspect <container> --format 'OOM={{.State.OOMKilled}} Exit={{.State.ExitCode}} Restarts={{.RestartCount}}'
# Exit=137 几乎必 OOM
```

**修**：改 `.env.<env>` 对应 `_MEM` / `_HEAP` 变量，`./deploy.sh <env> restart <service>` 或重跑 up。

### 问 4：MySQL healthcheck unhealthy

**症状**：`docker compose ps` 显示 webook-mysql `(health: starting)` 很久没变 healthy，业务 depends_on 挂不起来。

**排查**：
```bash
docker inspect webook-mysql --format '{{range .State.Health.Log}}exit={{.ExitCode}} {{.Output}}{{end}}' | tail -3
```

**常见原因**：
- mysql 还在首次 init（40-60s 正常）→ 等
- init 中被 OOM kill 留下半损坏 volume（看到 "Access denied for user 'root'@'localhost' (using password: YES)" 即使密码对）
  - 清 volume：`docker compose -p webook-<env> down -v` 或 `./deploy.sh <env> nuke`

### 问 5：staging/prod 访问不了

**别访问错端口**。端口按 env 错开：

| env | nginx | grafana | prometheus | zipkin |
|-----|-------|---------|------------|--------|
| local / dev | 80 | 3001 | 9090 | 9411 |
| staging | **81** | **3011** | **9190** | **9412** |
| prod | **82** | **3021** | **9290** | **9413** |

先 `./deploy.sh list` 确认 staging/prod project 真在跑（container_name 冲突可能静默阻止）。

### 问 6：kafka Windows go run 连不上

**症状**：webook 本地跑（Windows）连 kafka 报 `dial tcp 127.0.0.1:9094: connect: connection refused` 或 metadata 错 `webook-kafka:9092` 解析失败。

**原因**：kafka 的 EXTERNAL listener advertised 地址不对。

**local 模式设置**：`.env.local` 里 `KAFKA_EXTERNAL_HOST=localhost`、`KAFKA_EXTERNAL_PORT=9094`，webook-core 连 `localhost:9094` 才能成功。

### 问 7：Grafana SMTP 告警邮件没收到

```bash
cd deploy/grafana
make ENV=dev test-email    # 先用 UI Test 按钮验证 SMTP 通
make ENV=dev test-email-real # 触发真告警（停 webook-core 90s）
```

如果 UI Test 都收不到：
- `.env.<env>` 的 `GRAFANA_SMTP_ENABLED=true`、`GRAFANA_SMTP_USER/PASSWORD` 填了没
- QQ 邮箱走 STARTTLS 587 端口（已默认）
- 首次改 `.env` 后 Grafana 容器要重建：`./deploy.sh <env> restart grafana`

### 问 8：查看具体 env 变量注入到容器了没

```bash
docker inspect webook-core --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E 'MYSQL|REDIS|APP_ENV'
```

### 问 9：compose config 展开后到底长啥样

调试变量替换问题时用：
```bash
cd deploy
docker compose -p webook-dev --env-file .env.dev -f docker-compose.yaml config | head -50
```

会输出展开后的完整 YAML，所有 `${VAR}` 都被替换，能看清实际 container 配置。

---

## 文件布局

```
deploy/
├── docker-compose.yaml              # 基础定义（业务 webook-core/chat/fe + 中间件 + 监控栈）
├── docker-compose.local.yaml        # local override：build 代码 + 暴露宿主端口
├── .env.{local,dev,staging,prod}(.example)
├── deploy.sh                        # ./deploy.sh <local|dev|staging|prod>
├── nginx/
│   ├── nginx.conf                   # 主配置（JSON 日志、gzip、限流）
│   └── conf.d/default.conf          # /api/chat/* → webook-chat / /api/* → webook-core / 其余 → fe
├── prometheus/
│   ├── prometheus.yml               # 容器内部署用（scrape webook-core / webook-chat 各 job）
│   ├── prometheus.local.yml         # Windows 独立跑 prometheus.exe 用
│   ├── rules/                       # Recording rules
│   │   ├── webook-services.rules.yml  # HTTP 三大指标 by(job) 预聚合
│   │   └── webook-jobs.rules.yml      # cron / lock 系统级
│   └── examples/                    # 参考示例（不加载）
├── grafana/
│   ├── provisioning/                # Grafana 自动装载（folder=Webook）
│   │   ├── datasources/             # prometheus + zipkin
│   │   ├── dashboards/              # services-overview / service-catalog / webook-overview / webook-ops / webook-tracing / webook-jobs
│   │   └── alerting/                # webook-core.yml / webook-chat.yml / webook-jobs.yml + contactpoints / policies
│   ├── examples/                    # 参考示例（不装载）
│   └── Makefile                     # make ENV=dev reload / test-email / restart
├── otel-collector/config.yaml       # OTLP → Zipkin（带 sending_queue + retry + file_storage 容灾）
└── README.md
```

---

## L2 K8s 迁移对照

`deploy/` 是 docker-compose 的世界。K8s 的 Helm chart / manifest 将来新开 `k8s/` 或 `helm/` 目录与之并列，不混在一起。

| L1 这里 | L2 K8s |
|---------|--------|
| compose project `webook-<env>` | namespace `<env>` |
| `.env.<env>` | Secret + ConfigMap |
| compose `environment:` 注入 | `envFrom.secretRef` / `envFrom.configMapRef` |
| container_name 冲突阻止并发 | namespace 隔离，天然可并发 |
| volume 按 project 隔离 | PVC 按 namespace 隔离 |
| `nginx` 反代 | Ingress Controller + Ingress 规则 |
| 固定端口错开（80/81/82） | Ingress host-based routing（真域名） |

详见 `C:\Go\notes\cicd-webook-roadmap.md`。
