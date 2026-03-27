// 对应后端 domain.ArticleStatus
export enum ArticleStatus {
  Unknown = 0,
  Unpublished = 1,
  Published = 2,
  Private = 3,
}

// 对应后端 domain.Article
export interface Article {
  Id: number;
  Title: string;
  Content: string;
  Author: {
    Id: number;
    Name: string;
  };
  Status: ArticleStatus;
  CreatedAt: string;
  UpdatedAt: string;
}

// POST /article/edit 和 POST /article/publish 共用
export interface EditArticleReq {
  id?: number; // 无 id 为新建，有 id 为编辑
  title: string;
  content: string;
}

// POST /article/withdraw
export interface WithdrawArticleReq {
  id: number;
}
