# Webook 前端

Next.js 16 App Router + React 19 + TypeScript 5 + Ant Design 5 + Tailwind 4

## 核心依赖

| 包 | 用途 |
|---|---|
| `next` 16.2 | 框架（App Router） |
| `react` 19.2 | UI 库 |
| `typescript` 5 | 类型系统 |
| `antd` 5.25 | UI 组件库 |
| `@ant-design/v5-patch-for-react-19` | antd React 19 兼容补丁 |
| `axios` 1.13 | HTTP 客户端 |
| `dayjs` | 日期处理 |
| `tailwindcss` 4 | 原子化 CSS（`@tailwindcss/postcss`） |

## 常用命令

```bash
npm run dev          # 启动开发服务器（localhost:5173）
npm run build        # 生产构建
npm run lint         # ESLint 检查
npm run lint:fix     # ESLint 自动修复
```

## 架构

```
路由 (app/) → 页面 (views/) → API (api/) → 后端 (localhost:8089)
                  ↓
          组件 (components/) + Hooks (hooks/)
```

### 路由结构

Next.js App Router，路由组分为两组：

- `(auth)/` — 公开页面（login、register），无 Layout 包裹
- `(main)/` — 需登录页面，由 `AuthGuard` + `AppLayout` 包裹

`app/` 目录只做路由映射，业务逻辑写在 `views/` 中。

## 目录结构

```
webook-fe/
├── app/                    # Next.js App Router（仅路由映射）
│   ├── layout.tsx          # 根 Layout
│   ├── globals.css         # Tailwind 4 + 全局样式 + @layer 声明
│   ├── (auth)/             # 公开路由组
│   │   ├── layout.tsx      # antd patch 导入
│   │   ├── login/          # /login
│   │   ├── register/       # /register
│   │   └── ...
│   └── (main)/             # 需登录路由组（AuthGuard + AppLayout）
│       ├── layout.tsx      # AuthGuard + AppLayout + antd patch
│       ├── page.tsx        # /（首页）
│       ├── user/           # /user/profile、/user/edit
│       └── article/        # /article/list、/article/edit、/article/edit/[id]
├── types/                  # 共享类型定义（对应后端 domain）
│   ├── common.ts           # Result<T>、PageReq、PageResult<T>
│   ├── user.ts             # Profile、LoginReq、RegisterReq 等
│   ├── article.ts          # Article、ArticleStatus、EditArticleReq 等
│   └── index.ts            # 统一导出
├── api/                    # API 请求函数 + 请求实例
│   ├── request.ts          # Axios 实例 + Token 拦截器 + 401 刷新
│   ├── user.ts             # 用户相关接口
│   └── article.ts          # 文章相关接口
├── utils/                  # 工具函数
│   └── token.ts            # Token 存取（localStorage 统一入口）
├── hooks/                  # 自定义 Hooks
│   ├── useAuth.ts          # 登录态管理、登出
│   └── useRequest.ts       # 通用异步请求 hook
├── components/             # 可复用组件
│   ├── layout/             # 布局组件
│   │   ├── AppLayout.tsx   # Tailwind flex 布局（h-screen）
│   │   ├── Header.tsx      # 桌面水平菜单 + 移动端 Drawer 抽屉
│   │   └── AuthGuard.tsx   # 路由守卫（useSyncExternalStore）
│   └── common/             # 通用 UI
│       └── Loading.tsx     # 加载态组件
├── views/                  # 页面（按业务模块分目录）
│   ├── user/
│   ├── article/
│   └── home.tsx
├── postcss.config.mjs      # Tailwind 4 PostCSS 配置
├── next.config.ts           # Next.js 配置
├── eslint.config.mjs        # ESLint 9 flat config + prettier
├── tsconfig.json            # @/* → ./*（扁平结构，无 src/）
└── .env.local               # NEXT_PUBLIC_API_BASE_URL
```

## 分层规则

| 层 | 位置 | 职责 | 规则 |
|---|---|---|---|
| 页面 | `views/` | 路由页面，组装组件 + 调用 API | 不含可复用逻辑 |
| 组件 | `components/` | UI 复用单元 | 无副作用，通过 props 传数据 |
| Hooks | `hooks/` | 状态逻辑复用 | 封装 API 调用 + 状态管理 |
| API | `api/` | 请求函数 | 一个模块一个文件，返回类型化数据 |
| Types | `types/` | TS 类型定义 | 和后端 domain 模型对应 |
| Utils | `utils/` | 纯函数工具 | 无副作用 |

### 依赖方向

```
页面 → 组件 / Hooks / API
Hooks → API
API → axios 实例
组件 → props only（不直接调 API）
```

## 认证

Token 存储统一在 `utils/token.ts`，刷新逻辑在 `api/request.ts`：
- Access token → `localStorage['access-token']` → `x-access-token` header
- Refresh token → `localStorage['refresh-token']` → `x-refresh-token` header
- 401 → 自动刷新 → 重试请求队列 → 刷新失败跳转 `/login`

## 样式方案

- **布局**：Tailwind 类（`h-screen flex flex-col overflow-hidden`），不用 antd Layout 组件
- **组件**：antd 组件（Menu、Table、Form、Card 等）
- **响应式**：Tailwind 断点 `md:768px`，移动端优先
  - Header：桌面水平菜单，移动端 Drawer 抽屉（`hidden md:flex` / `flex md:hidden`）
  - 文章列表：桌面 Table，移动端卡片列表
  - 表单：`layout='vertical'` 自适应
- **Tailwind + antd 共存**：globals.css 中 `@layer` 声明控制优先级
- **滚动条**：全局美化（6px 圆角灰色）

## API 命名规范

| 动词 | 含义 | 示例 |
|------|------|------|
| `find` | 单条查询 | `findProfile()` / `findArticle(id)` |
| `page` | 分页查询 | `pageArticles(params)` |
| `list` | 全量列表（不分页） | `listArticles()` |
| `create` / `edit` | 新建或编辑 | `editArticle(data)` |
| `update` | 更新 | `updateProfile(data)` |
| `delete` / `withdraw` | 删除或撤回 | `deleteArticle(id)` / `withdrawArticle(data)` |

**命名规则：**
- 业务查询/操作函数必须带实体名：`findArticle`、`pageArticles`、`editArticle`
- 基础认证接口不加实体名：`login`、`register`、`logout`、`loginSms`、`sendSmsCode`
- 页面通过 `import * as articleApi from '@/api/article'` 使用
- 返回类型必须标注：`axios.post<Result<T>>()`

## UI 交互规范

- 破坏性操作（删除、撤回）统一用 `Modal.confirm` 弹窗确认，不用 Popconfirm
- 删除最后一页最后一条数据时自动回退到上一页
- 表单校验用 antd `<Form>` + `rules`，密码确认用 `dependencies` + 自定义 validator

## 命名规范

| 类型 | 格式 | 示例 |
|------|------|------|
| 页面文件 | 小写下划线 | `login_sms.tsx` |
| 组件文件 | PascalCase | `ArticleCard.tsx` |
| Hook 文件 | camelCase | `useAuth.ts` |
| API 文件 | 小写模块名 | `user.ts` / `article.ts` |
| 类型文件 | 小写模块名 | `user.ts` / `article.ts` |
| 组件名 | PascalCase | `ArticleCard` |
| 函数名 | camelCase | `findProfile` / `pageArticles` |
| 类型名 | PascalCase | `Profile` / `EditArticleReq` |
| 常量 | UPPER_SNAKE | `ACCESS_KEY` |

## 编码约束

- 组件必须是函数组件 + Hooks，禁止 class 组件
- 禁止 `any` 类型（ESLint 当前关闭了此规则，新代码仍需遵守）
- 禁止 `console.log` 残留（开发调试用完即删）
- 表单用 Ant Design `<Form>` + `onFinish`，不手写 onChange
- 路由跳转用 `next/navigation` 的 `useRouter`
- 不在组件内直接写 `axios.get/post`，必须通过 `api/` 调用
- `useSearchParams` 必须在 `<Suspense>` 内使用（Next.js 16 要求）
- 样式优先用 Tailwind class，antd 组件保持默认样式

## 已知技术债

| 问题 | 位置 | 优先级 |
|------|------|--------|
| 后端部分接口返回纯文本 | `register`, `login` | 中 — 应统一为 Result JSON |
| localStorage 存 token | `utils/token.ts` | 低 — 有 XSS 风险，后续可改 httpOnly cookie |
| 无前端测试 | - | 低 — 后续按需补 |
