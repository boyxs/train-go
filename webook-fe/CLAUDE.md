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

- `(auth)/` — 公开页面（login、register、feed、article/[id]），无 AuthGuard
- `(main)/` — 需登录页面，由 `AuthGuard` + `AppLayout` 包裹

`app/` 目录只做路由映射，业务逻辑写在 `views/` 中。

## 目录结构

```
webook-fe/
├── app/                    # Next.js App Router（仅路由映射）
│   ├── layout.tsx          # 根 Layout
│   ├── globals.css         # Tailwind 4 + @layer + 滚动条
│   ├── (auth)/             # 公开路由组
│   └── (main)/             # 需登录路由组
├── types/                  # 共享类型（对应后端 domain）
├── constants/              # 常量（storage key、HTTP header 等）
├── api/                    # API 请求函数 + Axios 实例
├── utils/                  # 工具函数
├── hooks/                  # 自定义 Hooks
├── components/             # 可复用组件
│   ├── layout/             # AppLayout / Header / AuthGuard / PublicHeader
│   └── common/             # Loading 等通用组件
├── views/                  # 页面（按业务模块）
├── postcss.config.mjs      # Tailwind 4 PostCSS
├── eslint.config.mjs        # ESLint 9 flat config
└── tsconfig.json            # @/* → ./*
```

## 分层规则

| 层 | 位置 | 职责 | 规则 |
|---|---|---|---|
| 页面 | `views/` | 组装组件 + 调用 API | 不含可复用逻辑 |
| 组件 | `components/` | UI 复用单元 | 无副作用，props 传数据 |
| Hooks | `hooks/` | 状态逻辑复用 | 封装 API + 状态 |
| API | `api/` | 请求函数 | 一模块一文件，类型化返回 |
| Types | `types/` | TS 类型 | 对应后端 domain |
| Constants | `constants/` | 常量 | UPPER_SNAKE，按领域分文件（storage/http） |
| Utils | `utils/` | 纯函数 | 无副作用 |

**依赖方向：** 页面 → 组件/Hooks/API → Utils/Types。**组件不直接调 API。**

## 性能约束

### 渲染性能
- **Server Component 优先**：`app/` 目录下的 layout/page 默认是 Server Component，只在需要 state/effect/浏览器 API 时加 `'use client'`
- **禁止在渲染路径中做重计算**：`useMemo`/`useCallback` 只在有实际性能问题时使用，不预防性添加
- **列表渲染必须有 `key`**：用业务 ID（`article.Id`），不用数组 index
- **大列表分页**：禁止一次性渲染超过 100 条数据，必须分页

### 网络性能
- **API 请求集中在 `api/` 层**：禁止组件内直接写 `axios.get/post`
- **请求去重**：`useRequest` hook 内置 cancelled flag 防止竞态
- **401 自动刷新**：`api/request.ts` 拦截器处理，带请求队列去重，避免并发刷新
- **超时**：全局 10s 超时（`timeout: 10_000`）
- **分页请求参数校验**：前端传 page/pageSize，后端兜底校验

### 打包性能
- **Tailwind 4 + PostCSS**：Tree-shaking 自动生效，不需手动 purge
- **antd 按需加载**：antd v5 自带 tree-shaking，直接 `import { Button } from 'antd'`
- **禁止引入不使用的包**：每次 review 检查 `package.json` 是否有僵尸依赖
- **图片用 `next/image`**（如有图片需求）

## 安全约束

### XSS 防护
- React 默认转义 JSX 内的变量，**禁止使用 `dangerouslySetInnerHTML`**（除非在 layout.tsx 注入样式）
- 用户输入不直接拼接 HTML

### Token 安全
- Token 存 localStorage（当前方案），**页面关闭不清除**
- 刷新 Token 逻辑在 `api/request.ts`，401 时自动刷新
- 登出必须调 `tokenUtil.clear()` 清除双 Token
- **不把 Token 放在 URL 参数中**

### 接口安全
- 公开接口（`/article/reader/*`、`/login`、`/register`）不传 Token
- 认证接口由 AuthGuard 保护，未登录自动跳转 `/login?redirect=`
- 破坏性操作（删除、撤回）必须 `Modal.confirm` 二次确认

## 状态管理

当前无全局状态管理（无 Redux/Zustand），状态按以下方式管理：

| 状态类型 | 方案 | 位置 |
|---------|------|------|
| 认证状态 | `localStorage` + `useSyncExternalStore` | `utils/token.ts` + `AuthGuard` |
| 页面数据 | `useRequest` hook | 各 `views/` 页面 |
| 表单状态 | `antd Form.useForm()` | 各表单页面 |
| UI 状态（弹窗/抽屉） | `useState` | 组件内部 |
| 跨 tab 同步 | `storage` 事件监听 | `AuthGuard`、`PublicHeader` |

**规则：能用局部状态解决的，不上全局。** 后续如需全局状态，优先 Zustand（轻量）。

## 样式约束

### Tailwind + antd 共存
- `globals.css` 中 `@layer tw-base, antd, tw-utilities` 控制优先级
- 布局用 Tailwind 类（`h-screen flex flex-col overflow-hidden`），不用 antd Layout
- antd 组件内部样式用 `style={{}}` 覆盖（antd CSS-in-JS 不接受 className）

### 响应式
- 断点：`md:768px`，移动端优先
- Header：桌面 Menu + 移动端 Drawer（`hidden md:flex` / `flex md:hidden`）
- Table：桌面 Table + 移动端卡片（`hidden md:block` / `block md:hidden`）
- 表单：`layout='vertical'` 自适应
- 公开页面用 `h-screen flex flex-col overflow-hidden` 锁定视口，内容区 `flex-1 overflow-auto`

### 设计规范
- **不用泛化的 AI 风格**：避免 Inter/Arial/Roboto、紫色渐变白底等通用配色
- **配色**：主色 + 锐利强调色，不用均匀弱色板
- **间距**：Tailwind spacing scale（4px 为基准），保持一致
- **圆角**：统一 `rounded-lg`（8px）
- **阴影**：hover 时用 `shadow-md`，静态不加阴影

## API 命名规范

| 动词 | 含义 | 示例 |
|------|------|------|
| `find` | 单条查询 | `findProfile()` / `findArticle(id)` |
| `page` | 分页查询 | `pageArticles(params)` |
| `list` | 全量列表 | `listArticles()` |
| `edit` | 新建或编辑 | `editArticle(data)` |
| `update` | 更新 | `updateProfile(data)` |
| `delete` / `withdraw` | 删除/撤回 | `deleteArticle(id)` |

**规则：** 业务函数带实体名（`findArticle`），认证函数不带（`login`）。

## UI 交互规范

- 破坏性操作统一 `Modal.confirm`，不用 Popconfirm
- 删除最后一页最后一条时自动回退上一页
- 表单校验用 antd `rules`，密码确认用 `dependencies` + validator
- Loading 状态用 `<Loading />` 组件，不用裸 Spin
- 空状态用 antd `<Empty>`，配 CTA 按钮

## 命名规范

| 类型 | 格式 | 示例 |
|------|------|------|
| 页面文件 | 小写下划线 | `login_sms.tsx` |
| 组件文件 | PascalCase | `PublicHeader.tsx` |
| Hook 文件 | camelCase | `useAuth.ts` |
| API 文件 | 小写模块名 | `article.ts` |
| 组件名 | PascalCase | `AppLayout` |
| 函数名 | camelCase | `pageArticles` |
| 类型名 | PascalCase | `EditArticleReq` |
| 常量 | UPPER_SNAKE | `ACCESS_KEY` |

## 编码约束

- 函数组件 + Hooks，禁止 class 组件
- 禁止 `any`（新代码）
- 禁止 `console.log` 残留
- 禁止组件内直接调 `axios`
- `useSearchParams` 必须在 `<Suspense>` 内
- `useEffect` 内禁止同步 `setState`（React 19 规则）— 用 `useSyncExternalStore` 替代
- 样式优先 Tailwind class，antd 组件用 style 覆盖
