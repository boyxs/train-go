# 小微书（Webook）

全栈项目：Go 后端（`webook/`）+ React 前端（`webook-fe/`）。

## 工作方式

> **先设计再编码，先测试再提交，不确定就问。**

- 每次只做一件事，不要跨模块大范围改动
- 新增代码遵循所在模块的现有模式和风格
- 不要自作主张修改文件组织结构，变更前先确认
- 不要引入新依赖包而不说明原因和替代方案
- 用中文沟通，代码注释可中可英但需要简洁
- 不确定的事情主动问，不要猜测后自行决定
- 不一次性输出超 300 行代码
- 相同逻辑出现两次以上且可复用时，按情况抽成公共函数/组件/工具（后端提到 `pkg/` 或对应层，前端提到 `hooks/` 或 `utils/`）
- **错误处理零容忍**：后端禁止 `_ = err`，所有错误必须处理或显式向上传播；前端 API 调用必须有 `.catch` 或 try/catch，用户可感知的失败必须有提示
- **改 `.env.*.example` 后必须补全**：只有**敏感数据（密码/密钥/token/DSN 等）或你不确定的值**才留占位符（`${ENV}` / `your-xxx` / 空）并**明确提示用户手填**；其余非敏感、且你已知的值（IMAGE_TAG / APP_ENV / 端口 / 资源限制 / 各 env 对齐值等）**一律补上、不留空、不漏项**，与同组或其它 env 保持一致

## 侦察优先

触碰项目里已重复出现的模式前，必须先 grep 或 read 现成实现对齐，禁止自行发明：

| 动作 | 必做的侦察 |
|------|-----------|
| 新列表 / 分页 | `Grep Pagination views/` → 对齐 `views/article/list.tsx` |
| 滚动容器 | `globals.css` 是否已有全局规则；禁止逐文件 inline |
| CSS 布局 bug | DevTools 定位真正 overflow / scroll 生效的元素，禁止猜 `html` / `body` |
| `.pen` 原型修改 | `batch_get` 读目标 frame；做类似页面用 `C()` 复制，禁止 `I()` 从零搭 |
| 后端字段不生效 | grep 字段赋值链查是否被某层硬覆盖，禁止直接怀疑前端 |
| 新常量 / 枚举 | `internal/consts/` 或 `constants/` 是否已有同类，禁止平行新增 |

## Pencil 原型修改

1. 动手前 `get_editor_state` + `batch_get` 读结构
2. 做类似页面用 `C(源id, parent, {x,y,width,...})`，禁止从零 `I()` 搭
3. 不确定 `iconFontName` 合法性时先 `get_guidelines(category:"icons")` 查
4. 复制来的 frame 里硬编码尺寸（如进度条 `width`）必须按新 viewport 比例重算
5. pen 改完必须 `export_nodes` 覆盖 PNG + 更新 PRD.md 原型章节

## 动手前先出方案

所有代码 / 配置改动（含改版本号 / 调顺序）先给一句话方案等用户确认再动手。下列必做：

- 修 bug：先指明改哪个文件 / 元素 / 字段，为什么是那个
- 改 API 签名或 DB 结构：先说影响范围
- 原型大改：按骨架 → Tab → Top3 → 其余 → 分页分阶段确认，禁止一次画完

## 导航

```
work/
├── webook/          # Go 后端 → 详见 webook/CLAUDE.md
├── webook-fe/       # React 前端 → 详见 webook-fe/CLAUDE.md
├── prd/             # 各模块产品/设计文档 + 原型资产（PRD / .pen / 导出 PNG）
├── CLAUDE.md        # 本文件：全局协作规则
└── CHANGELOG.md     # 变更日志（日期降序）
```

- **后端 API 路由**：查看各 Handler 的 `RegisterRoutes` 方法（`internal/web/*.go`）
- **前端页面路由**：查看 `app/` 目录结构（`(auth)/` 公开、`(main)/` 需登录、`(chat)/` 聊天）
- **设计 / 产品文档落点（铁律）**：每个模块的 PRD、原型 `.pen`、导出 PNG 等**所有设计资产统一放 `prd/<模块>/`**（如 `prd/comment/`、`prd/article/`、`prd/chat/`），与已有 `.pen` 同目录集中管理。**禁止**散落到 `webook/<svc>/docs/`、前端代码目录或别处；Pencil 原型 `export_nodes` 的 PNG 也必须写入对应 `prd/<模块>/`

## 前后端协作规则

- **接口先行**: 新功能先定义 API（路径、请求、响应），前后端同步开发
- **统一响应格式**: `{ code: number, msg: string, data: any }`
- **认证约定**: 请求 `x-access-token`，响应 `x-refresh-token`
- **变更同步**: 接口变更必须前后端同时更新

## Commit 格式

```
type(scope): description
```
- type: `feat` / `fix` / `refactor` / `docs` / `chore` / `perf` / `test`
- scope 后端: `web` / `service` / `repository` / `dao` / `cache` / `ioc`
- scope 前端: `page` / `component` / `api` / `hook` / `style` / `config`
- **禁止 AI 合作者签名**：commit message / PR body / merge message 一律不得包含 `Co-Authored-By: Claude`、`Co-Authored-By: GPT`、`Co-Authored-By: <AI 名> <noreply@anthropic.com>`、`🤖 Generated with Claude Code` 等任何把 AI 伪装成真实开发者协作的标识。git 历史只记录真实人类作者，AI 协作信息只在 PR 描述正文里说明（如有必要），不进 commit trailer。已发现含此类签名的提交需在合并前 amend 移除

## CHANGELOG.md

```
## [日期] 功能/修复名称

**变更内容**: 一句话描述
**影响范围**: 涉及哪些模块/文件
**技术决策**: 为什么这样做（如果有取舍）
**待办**: 后续需要跟进的事项（如果有）
**会话**: 会话名称
**发布**: YYYY-MM-DD（上线后补填）
```

日期降序，新条目插入头部。不要主动全量读取，追加时只读头部几行确认格式。

新功能用 `/rename` 命名会话：`YYMMDD-模块-功能中文`。

## 发版流程

推送 `webook-core-v*.*.*` / `webook-chat-v*.*.*` / `webook-fe-v*.*.*` tag 后**必须同步更新** `deploy/.env.prod.example`：

| 字段 | 对应 tag | 示例 |
|------|---------|------|
| `CORE_IMAGE_TAG` | `webook-core-v*.*.*` | `CORE_IMAGE_TAG=1.1.0` |
| `CHAT_IMAGE_TAG` | `webook-chat-v*.*.*` | `CHAT_IMAGE_TAG=1.1.0` |
| `FE_IMAGE_TAG`   | `webook-fe-v*.*.*`   | `FE_IMAGE_TAG=1.1.0`   |

每个服务独立打 tag、独立发版（按需推哪个 tag 就只动哪个 IMAGE_TAG）。不更新 → `./deploy.sh prod` 仍拉旧镜像，等于没发版。dev/staging 用 `main-latest` 滚动 tag 不动；只有 prod 走语义化版本固定。

实际 `.env.prod`（gitignored）由部署者按 example 手工同步。

## 服务拆分 / 新增服务

新增或拆分服务（例如 webook-chat 从 webook-core 抽离）后，**下表全部配置必须同步**，缺一项视为半成品：

| 维度 | 文件 | 改什么 |
|------|------|--------|
| 应用配置 | `<service>/config/{local,dev,staging,prod,test}.yaml` | 5 份同构 + 服务差异点（otel.service_name / otel.sample_ratio / server.http.addr） |
| Wire DI | `<service>/wire.go` + `wire ./...` 重生成 | Provider Set + InitWebServer |
| Prometheus 抓取 | `deploy/prometheus/prometheus.yml` | 加 `job_name: <service>` + targets |
| Prometheus 录制规则 | `deploy/prometheus/rules/*.rules.yml` | 新服务若有 cron / lock 等模式同步 |
| Grafana 告警 | `deploy/grafana/provisioning/alerting/<service>.yml` | up / 5xx / P99 / goroutines（镜像现有模板，`{job="<service>"}` 限定本服务） |
| Grafana 看板 | `deploy/grafana/provisioning/dashboards/*.json` | services-overview 加多服务对比 + 各 dashboard 用 service variable |
| Docker compose | `deploy/docker-compose.yaml` | 服务定义 + healthcheck + nginx depends_on / upstream / 路由 |
| Nginx 反代 | `deploy/nginx/conf.d/default.conf` | upstream + location 路由规则 |
| 部署脚本 | `deploy/deploy.sh` | logs / restart 默认 service 名等假设 |
| 部署变量 | `deploy/.env.<env>` + `.env.<env>.example` | `IMAGE_TAG` / `APP_ENV` 拆分按服务前缀（`CORE_` / `CHAT_`） |
| CI workflow | `.github/workflows/<service>-ci.yml` | 镜像现有 workflow，`paths` / `paths-ignore` 与其他服务互斥 |
| Dockerfile | `<service>/Dockerfile` | 多阶段构建（context = `webook/` 仓根） |
| **Metric 命名** | 应用层 builder | 统一 `webook_<subsystem>_*`（如 `webook_http_*`、`webook_db_*`），**禁止** `webook_<service>_*`；service 区分靠 prometheus 自动注入的 `job` label |
| 文档 | `CHANGELOG.md` + `<service>/CLAUDE.md` | 拆分原因 / 技术决策 / 接入方式 |

review 收尾前用 `grep -rn '<旧服务名>'` 全仓扫一遍，确认上面 14 类全部同步。漏一项 → 监控盲区 / 启动失败 / 告警不及时 / CI 不触发 / 部署不一致。

## 注释风格

禁止用 `// ===` 或 `// ---` 做分隔线。区域分隔用 Makefile 风格：

```
# ── 区域名 ────────────────────────────────────────────────
```

## 记录

- 完成功能后追加 `CHANGELOG.md`
- 任务中途要中断 → handover 文件已实时维护，重开会话自动接续
