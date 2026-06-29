import type {
  Comment,
  CreateCommentReq,
  GetRepliesReq,
  ListCommentReq,
  PageResult,
  Result,
} from '@/types';

import axios from './request';

// POST /comment/list — 一级评论分页（hot/new），含 likeCnt/liked（core 聚合 interaction）
export function listComments(data: ListCommentReq) {
  return axios.post<Result<PageResult<Comment>>>('/comment/list', data);
}

// POST /comment/replies — 楼内回复懒加载
export function getReplies(data: GetRepliesReq) {
  return axios.post<Result<PageResult<Comment>>>('/comment/replies', data);
}

// POST /comment/create — 发表评论/回复（需登录）
export function createComment(data: CreateCommentReq) {
  return axios.post<Result<{ comment: Comment }>>('/comment/create', data);
}

// POST /comment/delete — 删除自己的评论（需登录）
export function deleteComment(id: number) {
  return axios.post<Result>('/comment/delete', { id });
}
