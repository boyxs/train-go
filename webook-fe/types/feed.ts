// 关注流类型（对应 core BFF FeedItemVO）

export interface FeedTag {
  id: number;
  name: string;
}

export interface FeedAuthor {
  id: number;
  nickname: string;
}

// 关注流卡片：文章 + 作者昵称 + 互动/评论计数 + 标签
export interface FeedItem {
  articleId: number;
  title: string;
  abstract: string;
  author: FeedAuthor;
  publishedAt: number;
  likeCnt: number;
  collectCnt: number;
  commentCnt: number;
  tags: FeedTag[];
}

// POST /feed/list — 游标分页请求（cursor 缺省=首页；limit 缺省 10、后端夹取 1..20）
export interface FeedListReq {
  cursor?: number;
  limit?: number;
}
