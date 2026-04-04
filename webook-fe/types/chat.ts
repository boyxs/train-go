export interface Conversation {
  id: number;
  title: string;
  createdAt: string;
  updatedAt: string;
}

export interface Message {
  id: number;
  conversationId: number;
  role: 'user' | 'assistant' | 'system';
  content: string;
  toolCalls?: string;
  tokenUsed?: number;
  createdAt: string;
}

// SSE 事件类型
export interface ChatDeltaEvent {
  content: string;
  type: 'text';
}

export interface ChatToolCallEvent {
  name: string;
  args: Record<string, unknown>;
}

export interface ChatDoneEvent {
  messageId: number;
  usage: { promptTokens: number; completionTokens: number };
}

export interface ChatErrorEvent {
  code: number;
  msg: string;
}
