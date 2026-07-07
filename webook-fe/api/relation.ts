import type {
  BlockItem,
  BlocklistReq,
  CursorList,
  FolloweeItem,
  FollowerItem,
  RelationListReq,
  RelationStat,
  Result,
} from '@/types';

import axios from './request';

// POST /relation/follow — 关注（需登录）
export function follow(followeeId: number) {
  return axios.post<Result>('/relation/follow', { followeeId });
}

// POST /relation/unfollow — 取关（需登录）
export function unfollow(followeeId: number) {
  return axios.post<Result>('/relation/unfollow', { followeeId });
}

// POST /relation/block — 拉黑（需登录，级联解除双向关注）
export function block(targetId: number) {
  return axios.post<Result>('/relation/block', { targetId });
}

// POST /relation/unblock — 取消拉黑（需登录，不恢复关注）
export function unblock(targetId: number) {
  return axios.post<Result>('/relation/unblock', { targetId });
}

// POST /relation/stat — 计数 + 关系态（公开，登录态可选）
export function getRelationStat(userId: number) {
  return axios.post<Result<RelationStat>>('/relation/stat', { userId });
}

// POST /relation/followees — 关注列表（游标，公开）
export function listFollowees(data: RelationListReq) {
  return axios.post<Result<CursorList<FolloweeItem>>>(
    '/relation/followees',
    data,
  );
}

// POST /relation/followers — 粉丝列表（游标，公开）
export function listFollowers(data: RelationListReq) {
  return axios.post<Result<CursorList<FollowerItem>>>(
    '/relation/followers',
    data,
  );
}

// POST /relation/blocklist — 黑名单列表（游标，需登录）
export function listBlocks(data: BlocklistReq) {
  return axios.post<Result<CursorList<BlockItem>>>('/relation/blocklist', data);
}
