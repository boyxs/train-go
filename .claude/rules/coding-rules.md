# 编码规则

## 1. 文件修改

- 修改源代码（`.go` `.tsx` `.ts` `.js`）必须使用 Edit 工具，禁止 `sed -i` / `awk` / `perl -i` 等文本替换
- Edit 前必须 Read 目标文件，不基于记忆或猜测修改
- 每次 Edit 后立即 build + lint 验证，不积累多文件后验证
- 批量重命名/替换超过 3 个文件时，逐文件 Edit 并逐个验证

## 2. antd 组件使用

- `message` / `notification` / `modal` 必须通过 `App.useApp()` 获取实例，禁止静态导入
- 布局层（layout.tsx）必须包裹 `<ConfigProvider><App>...</App></ConfigProvider>`
- 主题色通过 ConfigProvider `token.colorPrimary` 统一控制，组件内不硬编码色值
- 手动用色时通过 ConfigProvider 的 `theme.token` 获取，保持全局一致

## 3. 破坏性操作

- 删除、撤回等不可逆操作统一 `Modal.confirm` 弹窗确认
- 删除最后一页最后一条数据时自动回退上一页
- 弹窗 `okButtonProps: { danger: true }`，明确标识风险

## 4. 响应式

- 移动端优先：默认样式 = 手机，通过 `md:` 断点适配桌面
- Table 在移动端替换为卡片列表（`hidden md:block` / `block md:hidden`）
- Header 移动端用 Drawer 抽屉替代水平 Menu

## 5. API 调用

- 禁止组件内直接调 `axios`，必须通过 `api/` 层
- 返回类型必须标注：`axios.post<Result<T>>()`
- 命名规范：业务函数带实体名（`findArticle`），认证函数不带（`login`）
