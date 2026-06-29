// 对应后端 domain.Interaction
export interface Interaction {
  articleId: number;
  readCount: number;
  likeCount: number;
  collectCount: number;
  liked: boolean; // 当前用户是否已点赞
  collected: boolean; // 当前用户是否已收藏
}

// interaction 通用目标：biz 业务类型 + bizId 业务内主键（article→articleId、comment→commentId）
export interface InteractionTarget {
  biz: string;
  bizId: number;
}

// POST /interaction/like
export interface LikeReq extends InteractionTarget {
  liked: boolean; // true=点赞，false=取消
}

// POST /interaction/collect
export interface CollectReq extends InteractionTarget {
  collected: boolean; // true=收藏，false=取消
}
