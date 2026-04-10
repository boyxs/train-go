# Webook 前端

Next.js App Router + React 19 + TypeScript + Ant Design 5 + Tailwind 4

## 常用命令

```bash
npm run dev          # 开发服务器（localhost:5173）
npm run build        # 生产构建
npm run lint:fix     # ESLint 自动修复
```

## 导航

```
路由 (app/) → 页面 (views/) → API (api/) → 后端 (localhost:8089)
                  ↓
          组件 (components/) + Hooks (hooks/)
```

| 目录 | 职责 | 找什么来这里 |
|------|------|------------|
| `app/` | 路由映射（仅 layout/page） | 路由结构、路由组 |
| `views/` | 页面业务逻辑 | 页面实现 |
| `components/layout/` | 布局组件 | AppLayout / Header / AuthGuard |
| `components/common/` | 通用 UI | Loading 等 |
| `components/chat/` | 聊天组件 | ChatBubble（全局浮窗） |
| `hooks/` | 状态逻辑复用 | useChat / useConversations 等 |
| `api/` | 请求函数 + Axios 实例 | 一模块一文件 |
| `types/` | TS 类型 | 对应后端 domain |
| `constants/` | 常量 | UPPER_SNAKE，按领域分文件 |
| `utils/` | 纯函数 | token 工具等 |

**路由组：** `(auth)/` 公开 · `(main)/` 需登录（AuthGuard + AppLayout） · `(chat)/` 聊天（独立布局）

## 分层规则

- **依赖方向：** 页面 → 组件/Hooks/API → Utils/Types
- **组件不直接调 API**，通过 props 或 hooks
- **`app/` 只做路由映射**，业务逻辑写在 `views/`
- API 函数带实体名（`findArticle`），认证函数不带（`login`）

## 样式约束（Tailwind + antd 共存）

- `globals.css` 中 `@layer tw-base, antd, tw-utilities` 控制优先级
- 布局用 Tailwind，antd 组件内部用 `style={{}}` 覆盖
- 主题色通过 ConfigProvider `token.colorPrimary` 统一控制，组件内不硬编码色值
- 响应式移动端优先：默认样式 = 手机，`md:` 断点适配桌面
- Table 移动端替换为卡片（`hidden md:block` / `block md:hidden`）

## antd 使用规则

- `message` / `notification` / `modal` 必须通过 `App.useApp()` 获取，禁止静态导入
- 布局层必须 `<ConfigProvider><App>...</App></ConfigProvider>`
- 破坏性操作统一 `Modal.confirm`，`okButtonProps: { danger: true }`
- **Table 必须带边框**：所有 `<Table>` 必须加 `bordered` 属性，参考 `views/article/list.tsx`
- **文章链接新标签打开**：指向文章详情的链接（`/article/{id}`）统一 `target='_blank'`

## 命名规范

| 类型 | 格式 | 示例 |
|------|------|------|
| 组件文件/名 | PascalCase | `PublicHeader.tsx` |
| Hook 文件 | camelCase | `useAuth.ts` |
| API 文件 | 小写模块名 | `article.ts` |
| 类型名 | PascalCase | `EditArticleReq` |
| 常量 | UPPER_SNAKE | `ACCESS_KEY` |

## 编码约束

- 禁止组件内直接调 `axios`，必须通过 `api/` 层
- 禁止 `any`（新代码）、禁止 `console.log` 残留
- 禁止 `dangerouslySetInnerHTML`（除 layout.tsx 注入样式）
- `useSearchParams` 必须在 `<Suspense>` 内
- 列表渲染用业务 ID 作 `key`，不用数组 index
- Server Component 优先，只在需要 state/effect/浏览器 API 时加 `'use client'`
