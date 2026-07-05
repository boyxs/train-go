import type { Article, PageResult, Result } from '@/types';

import axios from './request';

export interface SearchReq {
  query: string;
  page?: number;
  size?: number;
}

// POST /search/article — 语义搜索文章
// 后端此路径被 HTTP 15s 超时豁免（embedding 最长 30s + ES 查询），前端单独放宽
export function searchArticles(params: SearchReq) {
  return axios.post<Result<PageResult<Article>>>('/search/article', params, {
    timeout: 60_000,
  });
}
