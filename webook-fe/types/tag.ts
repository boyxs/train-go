// 对应后端 web.suggestVO（GET /tag/suggest）
export interface TagSuggest {
  name: string;
  slug: string;
  refCount: number;
}

// 对应后端 web.tagVO — 命中 / 推荐标签（仅名字 + slug）
export interface Tag {
  name: string;
  slug: string;
}

// 对应后端 web.tagDetailVO（GET /tag/:slug）
export interface TagDetail {
  name: string;
  slug: string;
  description: string;
  refCount: number;
  followCount: number; // 关注此标签的用户数
  weeklyNewCount: number; // 近 7 天新增内容数（"本周新增"）
  isFollowing: boolean; // 当前登录用户是否已关注（未登录恒 false）
}

// 对应后端 web.followVO（POST/DELETE /tag/:slug/follow）
export interface FollowResult {
  isFollowing: boolean;
  followCount: number;
}

// 对应后端 web.facetVO — 搜索侧标签 facet（含命中数）
export interface TagFacet {
  name: string;
  slug: string;
  count: number;
}

// 对应后端 web.taggedArticleVO — 标签下文章 / 搜索命中（含标签名与互动计数）
export interface TaggedArticle {
  id: number;
  title: string;
  abstract: string;
  author: { id: number; name: string };
  category: string;
  tags: Tag[];
  readCnt: number;
  likeCnt: number;
  collectCnt: number;
  createdAt: number;
}

// 标签下文章排序：最新 / 最热
export type TagArticleSort = 'new' | 'hot';

// POST /tag/:slug/articles 请求
export interface TagArticlesReq {
  page?: number;
  size?: number;
  sort?: TagArticleSort;
}
