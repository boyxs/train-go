import type { AIClickDashboard, AIClickReq, Result } from '@/types';

import axios from './request';

// POST /ai/click — 记录 AI 文章卡片点击（fire-and-forget）
export function recordAIClick(data: AIClickReq) {
  return axios.post<Result>('/ai/click', data);
}

// POST /ai/dashboard — 获取 AI 引流数据看板
export function fetchAIDashboard() {
  return axios.post<Result<AIClickDashboard>>('/ai/dashboard');
}
