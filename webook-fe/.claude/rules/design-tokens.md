# 小微书 — 设计规范

## 主色系统

| Token | 值 | 用途 |
|-------|------|------|
| `colorPrimary` | `#0D9488` | 按钮、链接、导航激活、分页器、头像 |
| `colorSuccess` | `#22C55E` | 已发布状态、成功提示 |
| `colorWarning` | `#D97706` | 草稿状态、警告 |
| `colorError` | `#EF4444` | 删除、撤回、危险操作 |
| `colorInfo` | `#6366F1` | 仅自己可见状态、辅助信息 |

## 中性色

| Token | 值 | 用途 |
|-------|------|------|
| `bgPage` | `#F5F5F5` | 页面背景 |
| `bgCard` | `#FFFFFF` | 卡片/容器背景 |
| `textPrimary` | `#1A1A1A` | 标题、正文 |
| `textSecondary` | `#6B7280` | 摘要、描述 |
| `textTertiary` | `#9CA3AF` | 元信息、占位符 |
| `textDisabled` | `#D1D5DB` | 禁用状态 |
| `borderLight` | `#F3F4F6` | Header 底边框、分割线 |
| `borderDefault` | `#E5E7EB` | 按钮/输入框边框 |

## 圆角

| 场景 | 值 |
|------|------|
| 卡片/容器 | `12px` |
| 按钮/输入框 | `8px` |
| 状态 Tag | `12px`（pill） |
| 头像 | `50%`（圆形） |

## 间距

| 场景 | 值 |
|------|------|
| 页面 padding | `16px 24px`（桌面），`12px`（移动） |
| 卡片内 padding | `20px 24px` |
| 卡片间距 | `12px` |
| 表单字段间距 | `20px` |

## 字体

| 场景 | 大小 | 字重 |
|------|------|------|
| 页面标题 | `20px` | `700` |
| 卡片标题 | `16px` | `700` |
| 正文/摘要 | `14px` | `400` |
| 元信息 | `12px` | `500` |
| 按钮文字 | `14px` | `600` |

## 状态色映射

| 文章状态 | 色值 | 背景 |
|---------|------|------|
| 已发布 | `#22C55E` | `#F0FDF4` |
| 草稿 | `#D97706` | `#FFFBEB` |
| 仅自己可见 | `#6366F1` | `#E0E7FF` |

## 交互

- hover 卡片：背景 `#FAFAFA`，标题变主色
- 破坏性操作：`Modal.confirm` 弹窗，`danger` 红色按钮
- 分页器：`showTotal` + `showSizeChanger` + `showQuickJumper`

## 响应式断点

| 断点 | 宽度 | 布局 |
|------|------|------|
| 移动端 | `< 768px` | 单列、Drawer 菜单、卡片列表 |
| 桌面端 | `≥ 768px` | 多列、水平 Menu、Table |

## antd ConfigProvider

```tsx
<ConfigProvider theme={{ token: { colorPrimary: '#0D9488' } }}>
```

所有 antd 组件自动跟随主色。手动用色时引用本文件的 Token 值。
