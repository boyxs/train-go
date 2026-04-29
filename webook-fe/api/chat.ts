import type {
  ChatDoneEvent,
  ChatErrorEvent,
  ChatToolCallEvent,
  Conversation,
  Message,
} from '@/types/chat';
import type { Result } from '@/types/common';
import { tokenUtil } from '@/utils/token';

import request from './request';

// POST /chat/conversation/create — 新建对话
export function createConversation() {
  return request.post<Result<Conversation>>('/chat/conversation/create');
}

// POST /chat/conversation/list — 对话列表
export function listConversations() {
  return request.post<Result<Conversation[]>>('/chat/conversation/list');
}

// POST /chat/conversation/delete — 删除对话
export function deleteConversation(conversationId: number) {
  return request.post<Result<null>>('/chat/conversation/delete', {
    conversationId,
  });
}

// POST /chat/message/list — 消息列表（分页）
export function listMessages(
  conversationId: number,
  beforeId?: number,
  limit?: number,
) {
  return request.post<Result<Message[]>>('/chat/message/list', {
    conversationId,
    beforeId: beforeId ?? 0,
    limit: limit ?? 20,
  });
}

// POST /chat/message/feedback — 设置消息反馈
export function setMessageFeedback(
  conversationId: number,
  messageId: number,
  feedback: number,
) {
  return request.post<Result<null>>('/chat/message/feedback', {
    conversationId,
    messageId,
    feedback,
  });
}

// POST /chat/message/stop — 停止生成
export function stopGeneration(conversationId: number) {
  return request.post<Result<null>>('/chat/stop', { conversationId });
}

// POST /chat/conversation/generating — 查询是否正在生成
export function isGenerating(conversationId: number) {
  return request.post<Result<boolean>>('/chat/conversation/generating', {
    conversationId,
  });
}

// SSE 回调
export interface SSECallbacks {
  onDelta: (text: string) => void;
  onToolCall?: (data: ChatToolCallEvent) => void;
  onToolResult?: (data: unknown) => void;
  onDone: (data: ChatDoneEvent) => void;
  onError: (err: ChatErrorEvent) => void;
}

// SSE fetch 用专属 base：dev 直连 chat 后端绕开 Next.js rewrites（dev rewrites 会 buffer 整个 body 破坏流式 — vercel/next.js#13146）。
// prod 不设 NEXT_PUBLIC_CHAT_SSE_URL，fallback 到 /api 走 nginx（已配 proxy_buffering off 真流式）。
const SSE_BASE =
  process.env.NEXT_PUBLIC_CHAT_SSE_URL ||
  process.env.NEXT_PUBLIC_API_BASE_URL ||
  '/api';

// SSE 超时：首字节 30s，流中 60s 无数据断开
const SSE_CONNECT_TIMEOUT = 30_000;
const SSE_STREAM_TIMEOUT = 60_000;

// SSE 发送消息（fetch + ReadableStream，不走 axios）
export function sendMessageSSE(
  conversationId: number,
  content: string,
  callbacks: SSECallbacks,
): AbortController {
  const controller = new AbortController();
  const token = tokenUtil.getAccess();
  let timedOut = false; // 区分超时 abort 和用户主动 abort

  // 首字节超时：30s 内未收到响应就中断
  const connectTimer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, SSE_CONNECT_TIMEOUT);

  fetch(`${SSE_BASE}/chat/message/send`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: token ? `Bearer ${token}` : '',
    },
    body: JSON.stringify({ conversationId, content }),
    signal: controller.signal,
  })
    .then(async (response) => {
      clearTimeout(connectTimer);

      if (!response.ok || !response.body) {
        callbacks.onError({ code: response.status, msg: '连接失败' });
        return;
      }

      // 后端业务错误返回 JSON（非 SSE），需要提前识别
      const contentType = response.headers.get('Content-Type') || '';
      if (!contentType.includes('text/event-stream')) {
        const json = await response.json();
        callbacks.onError({
          code: json.code ?? 0,
          msg: json.msg || '请求失败',
        });
        return;
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      // 流式读取超时：60s 无数据断开
      let streamTimer: ReturnType<typeof setTimeout> | undefined;
      const resetStreamTimer = () => {
        if (streamTimer) {
          clearTimeout(streamTimer);
        }
        streamTimer = setTimeout(() => {
          timedOut = true;
          controller.abort();
        }, SSE_STREAM_TIMEOUT);
      };
      resetStreamTimer();

      while (true) {
        const { done, value } = await reader.read();
        if (done) {
          if (streamTimer) {
            clearTimeout(streamTimer);
          }
          break;
        }
        resetStreamTimer();
        buffer += decoder.decode(value, { stream: true });

        // 解析 SSE 格式: event: xxx\ndata: {...}\n\n
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let currentEvent = '';
        for (const line of lines) {
          if (line.startsWith('event:')) {
            currentEvent = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            try {
              const data = JSON.parse(line.slice(5).trim());
              switch (currentEvent) {
                case 'delta':
                  callbacks.onDelta(data.content);
                  break;
                case 'tool_call':
                  callbacks.onToolCall?.(data.data ?? data);
                  break;
                case 'tool_result':
                  callbacks.onToolResult?.(data.data ?? data);
                  break;
                case 'done':
                  if (streamTimer) {
                    clearTimeout(streamTimer);
                  }
                  callbacks.onDone(data.data ?? data);
                  return;
                case 'error':
                  if (streamTimer) {
                    clearTimeout(streamTimer);
                  }
                  callbacks.onError(data.data ?? data);
                  return;
              }
            } catch {
              // 忽略非 JSON 行
            }
          }
        }
      }
    })
    .catch((err: Error) => {
      clearTimeout(connectTimer);
      if (err.name === 'AbortError' && timedOut) {
        callbacks.onError({ code: 0, msg: 'AI 响应超时，请重试' });
      } else if (err.name !== 'AbortError') {
        callbacks.onError({ code: 0, msg: '网络错误' });
      }
      // 用户主动 abort（stop）：不触发 onError，useChat 的 stop() 已处理状态
    });

  return controller;
}

// SSE 重连：从 Redis Stream 断点续传
// GET /chat/message/stream?conversationId=xx，header 带 Last-Event-ID
export function resumeStream(
  conversationId: number,
  lastEventId: string,
  callbacks: Partial<SSECallbacks> & {
    onStreamEnd?: () => void;
  },
): AbortController {
  const controller = new AbortController();
  const token = tokenUtil.getAccess();
  const url = `${SSE_BASE}/chat/message/stream?conversationId=${conversationId}`;

  fetch(url, {
    method: 'GET',
    headers: {
      Authorization: token ? `Bearer ${token}` : '',
      'Last-Event-ID': lastEventId || '0',
    },
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok || !response.body) {
        callbacks.onStreamEnd?.();
        return;
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let currentId = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let currentEvent = '';
        for (const line of lines) {
          if (line.startsWith('id:')) {
            currentId = line.slice(3).trim();
            // 暴露给上层调用者，下次重连用此 id 续传
            (callbacks as { onEventId?: (id: string) => void }).onEventId?.(
              currentId,
            );
          } else if (line.startsWith('event:')) {
            currentEvent = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            try {
              const data = JSON.parse(line.slice(5).trim());
              switch (currentEvent) {
                case 'delta':
                  callbacks.onDelta?.(data.content);
                  break;
                case 'tool_call':
                  callbacks.onToolCall?.(data.data ?? data);
                  break;
                case 'tool_result':
                  callbacks.onToolResult?.(data.data ?? data);
                  break;
                case 'done':
                  callbacks.onDone?.(data.data ?? data);
                  break;
                case 'error':
                  callbacks.onError?.(data.data ?? data);
                  break;
                case 'stream_end':
                  callbacks.onStreamEnd?.();
                  return;
              }
            } catch {
              // 忽略
            }
          }
        }
      }
      callbacks.onStreamEnd?.();
    })
    .catch((err: Error) => {
      // AbortError 是用户主动 / 组件 unmount，安静收尾；其他都是真错误，向上抛 onError
      if (err.name === 'AbortError') {
        callbacks.onStreamEnd?.();
        return;
      }
      callbacks.onError?.({ code: 0, msg: '断点重连失败' });
      callbacks.onStreamEnd?.();
    });

  return controller;
}
