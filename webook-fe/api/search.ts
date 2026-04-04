import type { Article, PageResult, Result } from '@/types';

import axios from './request';

export interface SearchReq {
  query: string;
  page?: number;
  size?: number;
}

// POST /search/article — 语义搜索文章
export function searchArticles(params: SearchReq) {
  return axios.post<Result<PageResult<Article>>>('/search/article', params);
}
