import type { CursorList, FeedItem, FeedListReq, Result } from '@/types';

import axios from './request';

// POST /feed/list — 关注流游标分页（需登录，uid 取自 JWT）
export function listFeed(data: FeedListReq) {
  return axios.post<Result<CursorList<FeedItem>>>('/feed/list', data);
}

// POST /feed/new-count — 自 sinceCursor 以来的新文章数（P1 提示条轮询，需登录）
export function feedNewCount(sinceCursor: number) {
  return axios.post<Result<{ count: number }>>('/feed/new-count', {
    sinceCursor,
  });
}
