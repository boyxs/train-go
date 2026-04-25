export interface Author {
  id: number;
  name: string;
}

export type Dimension = 'hot' | 'new' | 'best' | 'category';
export type Trend = 'new' | 'up' | 'down' | 'same';

export interface ArticleRanking {
  rank: number;
  articleId: number;
  title: string;
  author: Author;
  category: string;
  clicks: number;
  likes: number;
  collects: number;
  score: number;
  scoreRatio: number;
  trend: Trend;
  trendDelta: number;
}

export interface ArticleRankingPageReq {
  dimension: Dimension;
  category?: string;
  // YYYY-MM-DD，空字符串或不传视为今日
  date?: string;
  page?: number;
  pageSize?: number;
}

// 分页响应复用 types.PageResult<T>，此处无需重新定义

export interface ArticleRankingClickReq {
  articleId: number;
  rank: number;
  dimension: Dimension;
}
