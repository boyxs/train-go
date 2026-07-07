import type {
  Article,
  AuthorArticlesResult,
  EditArticleReq,
  PageReq,
  PageResult,
  Result,
  WithdrawArticleReq,
} from '@/types';

import axios from './request';

// POST /article/detail — 返回 Result<Article>
export function findArticle(id: number) {
  return axios.post<Result<Article>>('/article/detail', { id });
}

// POST /article/page — 分页查询，返回 Result<PageResult<Article>>
export function pageArticles(params: Partial<PageReq> = {}) {
  return axios.post<Result<PageResult<Article>>>('/article/page', params);
}

// POST /article/list — 全量列表，返回 Result<Article[]>
export function listArticles() {
  return axios.post<Result<Article[]>>('/article/list');
}

// POST /article/edit — 返回 Result<number>（文章 id）
export function editArticle(data: EditArticleReq) {
  return axios.post<Result<number>>('/article/edit', data);
}

// POST /article/publish — 返回 Result
export function publishArticle(data: EditArticleReq) {
  return axios.post<Result>('/article/publish', data);
}

// POST /article/withdraw — 返回 Result
export function withdrawArticle(data: WithdrawArticleReq) {
  return axios.post<Result>('/article/withdraw', data);
}

// POST /article/delete — 返回 Result
export function deleteArticle(id: number) {
  return axios.post<Result>('/article/delete', { id });
}

// POST /article/polish — AI 润色文章
export interface PolishResult {
  title: string;
  abstract: string;
  content: string;
}

// 后端 /article/polish 被 HTTP 15s 超时豁免（LLM 最长 60s），前端单独放宽，让后端超时先返错
export function polishArticle(data: { title: string; content: string }) {
  return axios.post<Result<PolishResult>>('/article/polish', data, {
    timeout: 70_000,
  });
}

// ===== 读者端（公开，不需要登录）=====

// POST /article/reader/page — 公开文章分页
export function pagePublishedArticles(params: Partial<PageReq> = {}) {
  return axios.post<Result<PageResult<Article>>>(
    '/article/reader/page',
    params,
  );
}

// POST /article/reader/detail — 公开文章详情
export function findPublishedArticle(id: number) {
  return axios.post<Result<Article>>('/article/reader/detail', { id });
}

// POST /article/reader/author — 某作者已发布文章 + 获赞总数（他人主页「TA 的文章」，公开）
export function pageAuthorArticles(data: {
  authorId: number;
  page?: number;
  pageSize?: number;
}) {
  return axios.post<Result<AuthorArticlesResult>>(
    '/article/reader/author',
    data,
  );
}
