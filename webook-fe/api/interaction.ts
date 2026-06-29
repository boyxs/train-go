import type {
  CollectReq,
  Interaction,
  InteractionTarget,
  LikeReq,
  Result,
} from '@/types';

import axios from './request';

// interaction 按 (biz, bizId) 通用：所有业务（article/comment/…）统一走这套，按 biz 区分。

// POST /interaction/view — 浏览上报（公开，不需要登录）
export function recordView(data: InteractionTarget) {
  return axios.post<Result>('/interaction/view', data);
}

// POST /interaction/detail — 获取互动聚合计数（公开，不含用户状态）
export function findInteraction(data: InteractionTarget) {
  return axios.post<Result<Interaction>>('/interaction/detail', data);
}

// POST /interaction/state — 获取当前用户的互动状态（需登录）
export function findUserState(data: InteractionTarget) {
  return axios.post<Result<{ liked: boolean; collected: boolean }>>(
    '/interaction/state',
    data,
  );
}

// POST /interaction/like — 点赞或取消（需登录）
export function like(data: LikeReq) {
  return axios.post<Result>('/interaction/like', data);
}

// POST /interaction/collect — 收藏或取消（需登录）
export function collect(data: CollectReq) {
  return axios.post<Result>('/interaction/collect', data);
}
