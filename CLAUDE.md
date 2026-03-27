# 小微书（Webook）

全栈项目：Go 后端（`webook/`）+ React 前端（`webook-fe/`）。

开发流程由 **workflow 插件**提供（`/workflow:architect` `/workflow:tdd` `/workflow:review` `/workflow:done` 等），安装：`/plugin marketplace add boyxs/workflow`

## 项目结构

```
work/
├── webook/          # Go 后端（Gin + GORM + Redis）
├── webook-fe/       # React 前端（Next.js + Ant Design）
├── CLAUDE.md        # 本文件：共享规则
└── DEVLOG.md        # 共享变更日志
```

## Agent Teams 协作

启动方式：在 `work/` 目录下启动 Claude Code，使用 `--add-dir` 添加子项目。

| 角色 | 工作目录 | 职责 |
|------|---------|------|
| 主 agent | `work/` | 需求拆分、协调前后端、API 联调 |
| 后端 agent | `webook/` | Go 代码、数据库、缓存、接口实现 |
| 前端 agent | `webook-fe/` | React 页面、组件、API 对接、样式 |

### 协作规则

- **接口先行**: 新功能先由后端 agent 定义 API（路径、请求、响应），前端 agent 再对接
- **统一响应格式**: 后端返回 `{ code: number, msg: string, data: any }`，前端统一按此解析
- **认证约定**: 前端在 `x-access-token` header 带 JWT，后端在 `x-refresh-token` 返回刷新 token
- **变更同步**: 接口变更必须同时更新前后端，不允许单方面改接口不通知

## API 约定

| Header | 用途 |
|--------|------|
| `x-access-token` | 请求携带 Access Token |
| `x-refresh-token` | 响应返回 Refresh Token |

后端基地址: `http://localhost:8089`（开发环境）

### 已实现接口

| Method | 路径 | 说明 | 认证 |
|--------|------|------|------|
| POST | `/user/register` | 邮箱注册 | 否 |
| POST | `/user/login` | 邮箱登录 | 否 |
| POST | `/user/login_sms/code/send` | 发送短信验证码 | 否 |
| POST | `/user/login_sms` | 短信登录 | 否 |
| GET | `/oauth2/wechat/authurl` | 微信授权 URL | 否 |
| ANY | `/oauth2/wechat/callback` | 微信回调 | 否 |
| POST | `/user/logout` | 登出 | JWT |
| POST | `/user/edit` | 编辑个人资料 | JWT |
| GET | `/user/profile` | 获取个人资料 | JWT |
| GET | `/user/refresh_token` | 刷新 Token | JWT |
| POST | `/article/edit` | 编辑文章（创建/更新草稿） | JWT |
| POST | `/article/publish` | 发布文章（制作库+线上库） | JWT |
| POST | `/article/withdraw` | 撤回文章（幂等） | JWT |
| POST | `/article/detail` | 文章详情（作者视角） | JWT |
| POST | `/article/list` | 文章列表（作者视角，分页） | JWT |

## Commit 格式

```
type(scope): description
```
- type: `feat` / `fix` / `refactor` / `docs` / `chore` / `perf` / `test`
- scope 后端: `web` / `service` / `repository` / `dao` / `cache` / `ioc`
- scope 前端: `page` / `component` / `api` / `hook` / `style` / `config`

## DEVLOG

完成功能后前插到 `work/DEVLOG.md`（日期降序）：

```
## YYYY-MM-DD
### 功能名称
**变更内容**: 一句话
**影响范围**: 前端/后端/全栈 + 涉及模块
**技术决策**: 为什么这样做
**待办**: 后续跟进
**会话**: YYMMDD-模块-功能
```

## 会话管理

- 新功能用 `/rename` 命名，格式：`YYMMDD-模块-功能中文`
- DEVLOG 带 **会话** 字段，方便 `/resume` 恢复上下文

## 沟通

- 用中文，代码注释可中可英
- 不确定主动问，不猜
- 不一次性输出超 300 行代码
- 不自作主张改文件结构，先确认
- 不引入新依赖而不说明原因
