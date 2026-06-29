// 互动业务类型，对应后端 domain.Biz*；interaction 按 (biz, bizId) 通用，按 biz 区分业务。
export const BIZ = {
  ARTICLE: 'article',
  COMMENT: 'comment',
} as const;

export type Biz = (typeof BIZ)[keyof typeof BIZ];
