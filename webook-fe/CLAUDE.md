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
路由 (app/) → 页面 (views/) → API (api/) → 后端 (localhost:8010)
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
- **实现必须读 pen 原型**：用 Pencil MCP `batch_get` 读取 `.pen` 文件的精确属性（fill、fontSize、padding、gap、cornerRadius 等），禁止看 PNG 截图目测样式

## 命名规范

| 类型 | 格式 | 示例 |
|------|------|------|
| 组件文件/名 | PascalCase | `PublicHeader.tsx` |
| Hook 文件 | camelCase | `useAuth.ts` |
| API 文件 | 小写模块名 | `article.ts` |
| 类型名 | PascalCase | `EditArticleReq` |
| 常量 | UPPER_SNAKE | `ACCESS_KEY` |
| API 返回数组 | `xxxList` | `const articleList = data.articles` |
| 自定义数组 | `xxxs` | `const ids = articleList.map(a => a.id)` |
| 索引/映射 | `xxxMap` | `const authorMap: Record<number, Author> = {}` |

## 编码约束

- 禁止组件内直接调 `axios`，必须通过 `api/` 层
- 禁止 `any`（新代码）、禁止 `console.log` 残留
- 禁止 `dangerouslySetInnerHTML`（除 layout.tsx 注入样式）
- `useSearchParams` 必须在 `<Suspense>` 内
- 列表渲染用业务 ID 作 `key`，不用数组 index
- Server Component 优先，只在需要 state/effect/浏览器 API 时加 `'use client'`

## 列表分页状态规则

**所有分页列表（榜单、文章、搜索结果等）必须把 `page`/`pageSize` 以及任何会影响列表数据的筛选/排序/tab/日期等状态写入 URL 查询参数**，参考 `views/article/list.tsx`、`views/search/RankingBoard.tsx`。`useState` 只用于纯 UI 状态（Drawer 开合、loading 等）。

**触发数据变化的操作后，页码处理规则**：

| 操作 | 页码行为 | 说明 |
|------|---------|------|
| **刷新浏览器** | 保留当前页 | URL 参数是真相，React state 从 `searchParams` 读取初值 |
| **编辑一条** | 不变 | 编辑不影响列表长度 / 顺序（若排序受字段影响需看情形） |
| **删除一条** | 默认不变；**若当前页只剩被删这一条且 `page > 1` → 回退到 `page - 1`** | 避免删完后停在空白页。参考 `article/list.tsx` 的 `refreshAfterRemove` |
| **新增一条** | 若有分页，**跳到新数据所在页**；若列表按新增时间倒序且新数据会进第 1 页 → 回到 `page = 1` | 不是无脑回 1；先判断新数据落在哪一页 |
| **切 tab / 换筛选** | 重置到 `page = 1` | 筛选条件变了再留旧页码无意义 |

**实现约定**：
- 默认值（`page=1`、`pageSize=20` 等）从 URL 参数中**移除**而非显式写入，保持 URL 干净
- 同一页面内多个列表共存时用前缀区分参数名（如榜单用 `rpage`/`rsize`，避免和搜索页 `page`/`size` 撞名）
- 统一用 `router.replace(...)` 更新 URL（不是 `router.push`），避免污染浏览器历史栈
- 不要用 `useEffect` 同步 state → URL，直接在事件处理器里 `setParams()` 更新 URL，state 自然从 URL 派生

## 跨页面模式对齐（grep 先行）

加跨页面重复出现的 UI 组件 / 交互模式前，必须先 grep 现有实现对齐，禁止自创参数组合：

| 要加的东西 | grep | 对齐文件 |
|-----------|------|---------|
| 分页列表 | `Grep "Pagination" views/` | `views/article/list.tsx` |
| Modal 确认 | `Grep "modal.confirm"` | `views/article/list.tsx` |
| Loading 占位 | `Grep "Skeleton\|Spin"` | `views/article/list.tsx` |
| URL 状态同步 | `Grep "useSearchParams"` | `views/article/list.tsx` |
| 空态 | `Grep "<Empty"` | `views/article/list.tsx` |
| 滚动容器 | `Grep "overflow-"` | `app/globals.css` |

## React Effect 依赖稳定性

Effect / useCallback 依赖数组禁止放每 render 新建的引用（数组 / 对象字面量）：

```tsx
// ❌ list = res?.data?.list ?? [] 每 render 新引用
React.useEffect(() => { ... }, [loading, list]);

// ✅ 用标量
React.useEffect(() => { ... }, [loading, list.length]);

// ✅ 或 useMemo 稳定引用
const stableList = React.useMemo(() => list, [list]);
React.useEffect(() => { ... }, [loading, stableList]);
```

## 常量集中 / 消灭魔数

UI 决策常量（阈值、默认 tab、pageSize、颜色 token、延迟时长）必须抽到文件顶部命名常量，禁止在 handler / inline style 中用字面量。多处使用的默认值集中定义：

```tsx
const DEFAULTS = { dim: 'hot', cat: 'tech', page: 1, pageSize: 20 } as const;
const LOADING_CLEAR_DELAY = 200;
```

## CSS 规则就近 vs 全局

相同 CSS 规则在 ≥ 2 处组件里重复（如 `scrollbar-gutter`、通用 hover 效果）必须抽到 `globals.css`，用 `@layer tw-utilities` 压过 Tailwind 默认，禁止每组件 inline style 复制。

改 CSS 布局 bug 前必须先用 DevTools 定位真正 overflow / scroll 生效的元素，禁止猜 `html` / `body`。
