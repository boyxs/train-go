// 对应后端 domain.Interaction
export interface Interaction {
  articleId: number;
  readCount: number;
  likeCount: number;
  collectCount: number;
  liked: boolean; // 当前用户是否已点赞
  collected: boolean; // 当前用户是否已收藏
}

// POST /interaction/like
export interface LikeReq {
  articleId: number;
  liked: boolean; // true=点赞，false=取消
}

// POST /interaction/collect
export interface CollectReq {
  articleId: number;
  collected: boolean; // true=收藏，false=取消
}
