import type { CollectReq, Interaction, LikeReq, Result } from '@/types';

import axios from './request';

// POST /interaction/view — 浏览上报（公开，不需要登录）
export function recordView(articleId: number) {
  return axios.post<Result>('/interaction/view', { articleId });
}

// POST /interaction/detail — 获取互动详情（公开，登录后有个人状态）
export function findInteraction(articleId: number) {
  return axios.post<Result<Interaction>>('/interaction/detail', { articleId });
}

// POST /interaction/like — 点赞或取消（需登录）
export function likeArticle(data: LikeReq) {
  return axios.post<Result>('/interaction/like', data);
}

// POST /interaction/collect — 收藏或取消（需登录）
export function collectArticle(data: CollectReq) {
  return axios.post<Result>('/interaction/collect', data);
}
