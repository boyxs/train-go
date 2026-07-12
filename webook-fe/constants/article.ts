// 文章分区候选。后端 category 为自由字符串、无枚举来源，此处 FE 维护一份小集供发文选择，可按需增删。
export const ARTICLE_CATEGORIES = [
  '技术',
  '生活',
  '职场',
  '阅读',
  '其他',
] as const;
