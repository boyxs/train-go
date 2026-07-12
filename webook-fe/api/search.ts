import type { Result, TaggedArticle, TagFacet } from '@/types';

import axios from './request';

export interface SearchReq {
  query: string;
  page?: number;
  size?: number;
  filter?: { tags?: string[] }; // 按标签 slug 过滤（多选 AND）
}

// 搜索响应：命中文章（含标签名与互动计数）+ 总数 + 标签 facet（含命中数）
export interface SearchArticlesResult {
  list: TaggedArticle[];
  total: number;
  facets: TagFacet[];
}

// POST /search/article — 语义搜索文章 + 标签 facet 过滤
// 后端此路径被 HTTP 15s 超时豁免（embedding 最长 30s + ES 查询），前端单独放宽
export function searchArticles(params: SearchReq) {
  return axios.post<Result<SearchArticlesResult>>('/search/article', params, {
    timeout: 60_000,
  });
}
