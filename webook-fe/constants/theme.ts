import type { ThemeConfig } from 'antd';

// 全站配色单一真相源（JS/antd/inline 侧）；与 globals.css @theme 同值双写，改色两处同步。
export const PALETTE = {
  primary: '#0D9488', // 主色 teal
  primaryDark: '#0B8178', // 主色加深（hover/active）
  tealSurface: '#F0FDFA', // 浅 teal 底：chip / 选中项 / 关注按钮
  tealBorder: '#99F6E4', // teal 描边
  ink: '#1A1A1A', // 文本主
  muted: '#6B7280', // 文本次（摘要/描述）
  subtle: '#9CA3AF', // 文本三级（元信息/占位）
  faint: '#D1D5DB', // 禁用
  hairline: '#F3F4F6', // 分割线/浅边框
  line: '#E5E7EB', // 默认边框
  page: '#F5F5F5', // 页面底
  surface: '#FFFFFF', // 卡片/容器底 + 白字
  surfaceHover: '#FAFAFA', // hover 底
  success: '#22C55E', // 成功/已发布
  successSurface: '#F0FDF4', // 成功底
  warning: '#D97706', // 警告/草稿
  warningSurface: '#FFFBEB', // 警告底
  danger: '#EF4444', // 错误/危险
  dangerSurface: '#FEF2F2', // 错误底
  info: '#6366F1', // 信息/仅自己可见
  infoSurface: '#E0E7FF', // 信息底
} as const;

// ANTD_THEME：各 layout 的 ConfigProvider 共用基座。
// Select 下拉「已选项」用浅 teal，替掉 antd 默认从主色推导的灰绿选中底色。
export const ANTD_THEME: ThemeConfig = {
  token: { colorPrimary: PALETTE.primary },
  components: {
    Select: {
      optionSelectedBg: PALETTE.tealSurface,
      optionSelectedColor: PALETTE.primary,
    },
  },
};
