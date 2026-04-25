import type { PageResult, Result } from '@/types';
import type {
  ArticleRanking,
  ArticleRankingClickReq,
  ArticleRankingPageReq,
} from '@/types/ranking';

import axios from './request';

// POST /article/ranking/page — 分页获取文章榜单
export function pageArticleRanking(body: ArticleRankingPageReq) {
  return axios.post<Result<PageResult<ArticleRanking>>>(
    '/article/ranking/page',
    body,
  );
}

// POST /article/ranking/archive/dates — 列出归档日期字符串数组
export function listArticleRankingArchiveDates() {
  return axios.post<Result<string[]>>('/article/ranking/archive/dates', {});
}

// POST /article/ranking/click — 上报榜单点击
export function reportArticleRankingClick(body: ArticleRankingClickReq) {
  return axios.post<Result<null>>('/article/ranking/click', body);
}

// POST /article/ranking/archive — 手动触发归档（测试/运维）
export function archiveArticleRanking(body: { date?: string }) {
  return axios.post<Result<null>>('/article/ranking/archive', body);
}
