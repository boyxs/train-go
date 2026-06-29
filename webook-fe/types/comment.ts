// 对应后端 web.CommentVO（likeCnt/liked 由 core 聚合 interaction(biz="comment") 填入）
export interface Comment {
  id: number;
  user: CommentUser;
  content: string;
  rootId: number; // 根评论 id（一级评论=0）
  pid: number; // 直接父评论 id（一级=0）
  replyCnt: number; // 直接回复数
  likeCnt: number;
  liked: boolean; // 当前用户是否已点赞
  deleted: boolean; // 已删除占位（有子回复时保留，渲染「该评论已删除」）
  createdAt: number; // Unix 毫秒
  children?: Comment[]; // 一级评论携带的前 N 条回复预览（P0 走懒加载，通常为空）
}

export interface CommentUser {
  id: number;
  name: string;
}

// POST /comment/list — 一级评论分页
export interface ListCommentReq {
  articleId: number;
  sort: CommentSort;
  offset?: number;
  limit: number;
}

export type CommentSort = 'hot' | 'new';

// POST /comment/replies — 楼内回复懒加载
export interface GetRepliesReq {
  rootId: number;
  offset?: number;
  limit: number;
}

// POST /comment/create — 发表评论/回复
export interface CreateCommentReq {
  articleId: number;
  content: string;
  pid?: number; // 回复目标父评论；省略/0=一级评论
}
