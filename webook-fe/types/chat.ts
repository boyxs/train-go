export interface Conversation {
  id: number;
  title: string;
  createdAt: number;
  updatedAt: number;
}

export interface Message {
  id: number;
  conversationId: number;
  role: 'user' | 'assistant' | 'system';
  content: string;
  toolCalls?: string;
  tokenUsed?: number;
  createdAt: number;
}

// SSE 事件类型
export interface ChatDeltaEvent {
  content: string;
  type: 'text';
}

export interface ChatToolCallEvent {
  id: string;
  name: string;
  args: Record<string, unknown>;
}

export interface ArticleCard {
  id: number;
  title: string;
  abstract: string;
}

export interface ChatToolResultEvent {
  callId: string;
  name: string;
  articles?: ArticleCard[];
  error?: string;
}

export interface ChatDoneEvent {
  messageId: number;
  usage: { promptTokens: number; completionTokens: number };
}

export interface ChatErrorEvent {
  code: number;
  msg: string;
}

/** 消息中附带的工具状态（用于前端展示） */
export interface MessageToolState {
  callId: string;
  name: string;
  status: 'running' | 'done' | 'error';
  result?: ChatToolResultEvent;
}
