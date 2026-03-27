# 小微书（Webook）

全栈项目：Go 后端（`webook/`）+ React 前端（`webook-fe/`）。

开发流程由 **workflow 插件**提供（`/workflow:architect` `/workflow:tdd` `/workflow:review` `/workflow:done` 等），安装：`/plugin marketplace add boyxs/workflow`

## 项目结构

```
work/
├── webook/          # Go 后端（Gin + GORM + Redis）→ 详见 webook/CLAUDE.md
├── webook-fe/       # React 前端（Next.js + Ant Design）→ 详见 webook-fe/CLAUDE.md
├── CLAUDE.md        # 本文件：全局协作规则
└── DEVLOG.md        # 变更日志
```

## 前后端协作规则

- **接口先行**: 新功能先定义 API（路径、请求、响应），前后端同步开发
- **统一响应格式**: `{ code: number, msg: string, data: any }`
- **认证约定**: 请求 `x-access-token`，响应 `x-refresh-token`
- **变更同步**: 接口变更必须前后端同时更新

## API 总览

后端基地址: `http://localhost:8089`

### 用户模块

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

### 文章模块 — 作者端（需登录）

| Method | 路径 | 说明 | 认证 |
|--------|------|------|------|
| POST | `/article/edit` | 编辑文章（创建/更新草稿） | JWT |
| POST | `/article/publish` | 发布文章 | JWT |
| POST | `/article/withdraw` | 撤回文章 | JWT |
| POST | `/article/delete` | 永久删除文章 | JWT |
| POST | `/article/detail` | 文章详情（作者视角） | JWT |
| POST | `/article/page` | 文章分页（作者视角） | JWT |
| POST | `/article/list` | 文章全量列表 | JWT |

### 文章模块 — 读者端（公开）

| Method | 路径 | 说明 | 认证 |
|--------|------|------|------|
| POST | `/article/reader/page` | 公开文章分页 | 否 |
| POST | `/article/reader/detail` | 公开文章详情 | 否 |

## Commit 格式

```
type(scope): description
```
- type: `feat` / `fix` / `refactor` / `docs` / `chore` / `perf` / `test`
- scope 后端: `web` / `service` / `repository` / `dao` / `cache` / `ioc`
- scope 前端: `page` / `component` / `api` / `hook` / `style` / `config`

## DEVLOG

完成功能后前插到 `work/DEVLOG.md`（日期降序）。

## 沟通

- 用中文，代码注释可中可英
- 不确定主动问，不猜
- 不一次性输出超 300 行代码
- 不自作主张改文件结构，先确认
- 不引入新依赖而不说明原因
