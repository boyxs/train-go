import type { Tag } from './tag';

// 对应后端 domain.ArticleStatus
export enum ArticleStatus {
  Unknown = 0,
  Unpublished = 1,
  Published = 2,
  Private = 3,
}

// 对应后端 domain.Article
export interface Article {
  id: number;
  title: string;
  content: string;
  abstract: string;
  author: {
    id: number;
    name: string;
  };
  authorId?: number; // reader 端 VO 扁平返回作者 id（core web.ReaderDetailVO）
  status: ArticleStatus;
  readCnt?: number;
  category?: string; // 分区回显
  tags?: string[]; // 作者详情回显用（编辑器预填）
  createdAt: number;
  updatedAt: number;
}

// POST /article/edit 和 /article/publish
export interface EditArticleReq {
  id?: number;
  title: string;
  abstract?: string;
  content: string;
  category?: string; // 分区（可空）
  tags?: string[]; // 作者输入的标签名（≤5，发布时经 tag 服务归一；存草稿不落）
}

// POST /article/withdraw
export interface WithdrawArticleReq {
  id: number;
}

// 对应后端 web.ReaderArticleVO（/article/reader/author 他人主页「TA 的文章」）
export interface ReaderArticle {
  id: number;
  title: string;
  abstract: string;
  authorId: number;
  readCnt: number;
  likeCnt: number;
  commentCnt: number;
  createdAt: number;
  updatedAt: number;
}

// POST /article/reader/author 响应：作者已发布文章分页 + 获赞总数
export interface AuthorArticlesResult {
  list: ReaderArticle[];
  total: number;
  likedTotal: number; // 获赞：作者全部已发布文章的点赞聚合
}

// 对应后端 web.ReaderDetailVO（POST /article/reader/detail，公开阅读页）
// tags 为 {name,slug}（阅读页 chip 链 /tag/:slug），与作者端 EditArticleReq.tags(名字数组) 区分
export interface ReaderArticleDetail {
  id: number;
  title: string;
  content: string;
  abstract: string;
  authorId: number;
  readCnt: number;
  tags?: Tag[];
  createdAt: number;
  updatedAt: number;
}
