export interface AIClickReq {
  article_id: number;
  conversation_id: number;
}

export interface DailyTrend {
  date: string;
  clicks: number;
}

export interface TopArticle {
  rank: number;
  articleId: number;
  title: string;
  clicks: number;
  uniqueUsers: number;
}

export interface AIClickDashboard {
  totalClicks: number;
  uniqueUsers: number;
  uniqueArticles: number;
  avgClicksPerUser: number;
  dailyTrend: DailyTrend[];
  topArticles: TopArticle[];
}
