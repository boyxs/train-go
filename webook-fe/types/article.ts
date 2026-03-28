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
  status: ArticleStatus;
  createdAt: string;
  updatedAt: string;
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
