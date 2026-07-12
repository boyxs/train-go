import type {
  FollowResult,
  PageResult,
  Result,
  Tag,
  TagArticlesReq,
  TagDetail,
  TaggedArticle,
  TagSuggest,
} from '@/types';

import axios from './request';

// GET /tag/suggest?q=&limit= — typeahead 前缀补全已有标签（需登录）
export function suggestTags(q: string, limit = 10) {
  return axios.get<Result<TagSuggest[]>>('/tag/suggest', {
    params: { q, limit },
  });
}

// POST /tag/recommend — AI 基于标题+正文推荐候选标签（需登录）
// 后端此路径 HTTP 15s 超时豁免（embedding + ES kNN），前端单独放宽
export function recommendTags(data: { title: string; content: string }) {
  return axios.post<Result<Tag[]>>('/tag/recommend', data, {
    timeout: 60_000,
  });
}

// GET /tag/:slug — 标签详情（公开）
export function findTag(slug: string) {
  return axios.get<Result<TagDetail>>(`/tag/${encodeURIComponent(slug)}`);
}

// POST /tag/:slug/articles — 标签下文章分页（公开）
export function pageTagArticles(slug: string, params: TagArticlesReq = {}) {
  return axios.post<Result<PageResult<TaggedArticle>>>(
    `/tag/${encodeURIComponent(slug)}/articles`,
    params,
  );
}

// POST /tag/:slug/follow — 关注标签（需登录），返回翻转后关注态 + 关注数
export function followTag(slug: string) {
  return axios.post<Result<FollowResult>>(
    `/tag/${encodeURIComponent(slug)}/follow`,
  );
}

// DELETE /tag/:slug/follow — 取关标签（需登录）
export function unfollowTag(slug: string) {
  return axios.delete<Result<FollowResult>>(
    `/tag/${encodeURIComponent(slug)}/follow`,
  );
}
