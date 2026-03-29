// sessionStorage / localStorage key 统一管理
// 新增 key 时在此文件注册，避免散落各处导致冲突

export const STORAGE_KEYS = {
  // localStorage — 持久化
  ACCESS_TOKEN: 'access-token',
  REFRESH_TOKEN: 'refresh-token',

  // sessionStorage — 当前标签页生命周期
  SCROLL_FEED: 'scroll:feed',
  SCROLL_ARTICLE_LIST: 'scroll:article-list',
} as const;
