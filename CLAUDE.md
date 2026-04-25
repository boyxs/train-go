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
├── CLAUDE.md        # 本文件：全局协作规则
└── CHANGELOG.md     # 变更日志（日期降序）
```

- **后端 API 路由**：查看各 Handler 的 `RegisterRoutes` 方法（`internal/web/*.go`）
- **前端页面路由**：查看 `app/` 目录结构（`(auth)/` 公开、`(main)/` 需登录、`(chat)/` 聊天）

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

推送 `webook-v*.*.*` 或 `webook-fe-v*.*.*` tag 后**必须同步更新** `deploy/.env.prod.example`：

| 字段 | 对应 tag | 示例 |
|------|---------|------|
| `IMAGE_TAG` | `webook-v*.*.*` | `IMAGE_TAG=1.1.0` |
| `FE_IMAGE_TAG` | `webook-fe-v*.*.*` | `FE_IMAGE_TAG=1.1.0` |

不更新 → `./deploy.sh prod` 仍拉旧镜像，等于没发版。dev/staging 用 `master-latest` 滚动 tag 不动；只有 prod 走语义化版本固定。

实际 `.env.prod`（gitignored）由部署者按 example 手工同步。

## 注释风格

禁止用 `// ===` 或 `// ---` 做分隔线。区域分隔用 Makefile 风格：

```
# ── 区域名 ────────────────────────────────────────────────
```


## 记录

- 完成功能后追加 `CHANGELOG.md`
- 发现更好做法记录到 `memory/`（feedback 类型）
