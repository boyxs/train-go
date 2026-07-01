// 后端统一响应格式 — 对应 ginx.Result{Code, Reason, Msg, Data, Metadata}
// 注意：部分旧接口（register, login）返回纯文本，不走此结构
export interface Result<T = unknown> {
  code: number;
  reason?: string;
  msg: string;
  data: T;
  metadata?: Record<string, string>;
}

// 通用分页请求
export interface PageReq {
  page: number;
  pageSize: number;
}

// 通用分页响应
export interface PageResult<T> {
  list: T[];
  total: number;
}
