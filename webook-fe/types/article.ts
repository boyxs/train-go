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
  createdAt: number;
  updatedAt: number;
}

// POST /article/edit 和 /article/publish
export interface EditArticleReq {
  id?: number;
  title: string;
  abstract?: string;
  content: string;
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
