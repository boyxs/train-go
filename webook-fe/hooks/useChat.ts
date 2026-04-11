import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import * as chatApi from '@/api/chat';
import type {
  ChatToolResultEvent,
  Message,
  MessageToolState,
} from '@/types/chat';

/** 扩展 Message，附带工具调用状态（仅 pending 阶段使用） */
interface PendingMessage extends Message {
  toolStates?: MessageToolState[];
}

const PAGE_SIZE = 10;

/** 按 conversationId 缓存的 pending 状态 */
interface PendingBuffer {
  messages: PendingMessage[];
  streaming: boolean;
}

export function useChat(conversationId: number | null) {
  const [serverMessages, setServerMessages] = useState<Message[]>([]);
  const [pendingMessages, setPendingMessages] = useState<PendingMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const tempIdRef = useRef(-1);

  // 按 conversationId 缓存 pending 消息，支持切换对话不丢失进度
  const bufferRef = useRef<Map<number, PendingBuffer>>(new Map());
  const activeIdRef = useRef<number | null>(null);

  // 更新 buffer 并同步到 state（如果是当前活跃对话）
  const updateBuffer = useCallback(
    (convId: number, updater: (msgs: PendingMessage[]) => PendingMessage[]) => {
      const buf = bufferRef.current.get(convId);
      const oldMsgs = buf?.messages ?? [];
      const newMsgs = updater(oldMsgs);
      bufferRef.current.set(convId, {
        messages: newMsgs,
        streaming: buf?.streaming ?? false,
      });
      // 只在当前活跃对话时更新 React state
      if (activeIdRef.current === convId) {
        setPendingMessages(newMsgs);
      }
    },
    [],
  );

  const setBufferStreaming = useCallback((convId: number, value: boolean) => {
    const buf = bufferRef.current.get(convId);
    bufferRef.current.set(convId, {
      messages: buf?.messages ?? [],
      streaming: value,
    });
    if (activeIdRef.current === convId) {
      setStreaming(value);
    }
  }, []);

  // conversationId 变化时：恢复 buffer → 加载历史
  useEffect(() => {
    activeIdRef.current = conversationId;

    // 恢复目标对话的 buffer（如果有正在 streaming 的）
    const buf = conversationId
      ? bufferRef.current.get(conversationId)
      : undefined;
    const restoredPending = buf && buf.messages.length > 0 ? buf.messages : [];
    const restoredStreaming = buf?.streaming ?? false;

    // 使用 requestAnimationFrame 批量更新，避免 effect 内同步 setState
    requestAnimationFrame(() => {
      setError(null);
      setPendingMessages(restoredPending);
      setStreaming(restoredStreaming);
    });

    // 加载历史消息
    if (!conversationId) {
      requestAnimationFrame(() => {
        setServerMessages([]);
        setLoading(false);
      });
      return;
    }

    let cancelled = false;

    requestAnimationFrame(() => {
      setLoading(true);
    });

    const convId = conversationId;
    let pollTimer: ReturnType<typeof setInterval> | null = null;

    // 加载消息 + 检查是否在生成中
    Promise.all([
      chatApi.listMessages(convId, 0, PAGE_SIZE),
      chatApi.isGenerating(convId),
    ])
      .then(([msgRes, genRes]) => {
        if (cancelled) {
          return;
        }
        if (msgRes.data.code === 0) {
          const msgs = msgRes.data.data ?? [];
          setServerMessages(msgs);
          setHasMore(msgs.length >= PAGE_SIZE);
        }
        setLoading(false);

        // 后端正在生成 且 前端没有活跃的 buffer → 轮询 listMessages
        const hasBuffer = bufferRef.current.has(convId);
        const generating = genRes.data.data === true;
        if (generating && !hasBuffer) {
          setStreaming(true);
          pollTimer = setInterval(() => {
            if (cancelled) {
              return;
            }
            Promise.all([
              chatApi.listMessages(convId, 0, PAGE_SIZE),
              chatApi.isGenerating(convId),
            ]).then(([pollMsgRes, pollGenRes]) => {
              if (cancelled) {
                return;
              }
              if (pollMsgRes.data.code === 0) {
                setServerMessages(pollMsgRes.data.data ?? []);
              }
              if (pollGenRes.data.data !== true) {
                setStreaming(false);
                if (pollTimer) {
                  clearInterval(pollTimer);
                  pollTimer = null;
                }
              }
            });
          }, 1500);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError('加载消息失败');
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
      if (pollTimer) {
        clearInterval(pollTimer);
      }
    };
  }, [conversationId]);

  const messages = useMemo(() => {
    const restored: PendingMessage[] = serverMessages.map((msg) => {
      if (msg.role !== 'assistant' || !msg.toolCalls) {
        return msg;
      }
      try {
        const results: ChatToolResultEvent[] = JSON.parse(msg.toolCalls);
        const toolStates: MessageToolState[] = results.map((r) => ({
          callId: r.callId,
          name: r.name,
          status: (r.error ? 'error' : 'done') as MessageToolState['status'],
          result: r,
        }));
        return { ...msg, toolStates };
      } catch {
        return msg;
      }
    });
    // 去重：pending 里的消息优先（可能有 toolStates 等实时状态）
    const pendingIds = new Set(pendingMessages.map((m) => m.id));
    const deduped = restored.filter((m) => !pendingIds.has(m.id));
    return [...deduped, ...pendingMessages];
  }, [serverMessages, pendingMessages]);

  // 发送消息
  const send = useCallback(
    (content: string) => {
      if (!conversationId || !content.trim()) {
        return;
      }
      const convId = conversationId;

      const userMsg: Message = {
        id: tempIdRef.current--,
        conversationId: convId,
        role: 'user',
        content: content.trim(),
        createdAt: Date.now(),
      };
      const aiMsg: Message = {
        id: tempIdRef.current--,
        conversationId: convId,
        role: 'assistant',
        content: '',
        createdAt: Date.now(),
      };

      // 初始化 buffer
      bufferRef.current.set(convId, {
        messages: [userMsg, aiMsg],
        streaming: true,
      });
      setPendingMessages([userMsg, aiMsg]);
      setStreaming(true);
      setError(null);

      const controller = chatApi.sendMessageSSE(convId, content.trim(), {
        onDelta: (text) => {
          updateBuffer(convId, (prev) => {
            if (prev.length === 0) {
              return prev;
            }
            const updated = [...prev];
            const last = updated[updated.length - 1];
            if (last.role === 'assistant') {
              updated[updated.length - 1] = {
                ...last,
                content: last.content + text,
              };
            }
            return updated;
          });
        },
        onToolCall: (data) => {
          updateBuffer(convId, (prev) => {
            if (prev.length === 0) {
              return prev;
            }
            const updated = [...prev];
            const last = updated[updated.length - 1];
            if (last.role === 'assistant') {
              const prevStates = last.toolStates ?? [];
              updated[updated.length - 1] = {
                ...last,
                toolStates: [
                  ...prevStates,
                  { callId: data.id, name: data.name, status: 'running' },
                ],
              };
            }
            return updated;
          });
        },
        onToolResult: (rawData) => {
          const data = rawData as ChatToolResultEvent;
          updateBuffer(convId, (prev) => {
            if (prev.length === 0) {
              return prev;
            }
            const updated = [...prev];
            const last = updated[updated.length - 1];
            if (last.role === 'assistant' && last.toolStates) {
              updated[updated.length - 1] = {
                ...last,
                toolStates: last.toolStates.map((s) =>
                  s.callId === data.callId
                    ? {
                        ...s,
                        status: data.error ? 'error' : 'done',
                        result: data,
                      }
                    : s,
                ),
              };
            }
            return updated;
          });
        },
        onDone: (data) => {
          setBufferStreaming(convId, false);
          updateBuffer(convId, (prev) => {
            if (prev.length === 0) {
              return prev;
            }
            const updated = [...prev];
            const last = updated[updated.length - 1];
            if (last.role === 'assistant') {
              updated[updated.length - 1] = {
                ...last,
                id: data?.messageId ?? last.id,
                tokenUsed: data?.usage
                  ? data.usage.promptTokens + data.usage.completionTokens
                  : 0,
              };
            }
            return updated;
          });
          // 生成完成，清除 buffer
          setTimeout(() => {
            bufferRef.current.delete(convId);
          }, 1000);
          controllerRef.current = null;
        },
        onError: (err) => {
          setBufferStreaming(convId, false);
          if (activeIdRef.current === convId) {
            setError(err.msg || '生成失败');
          }
          updateBuffer(convId, (prev) => {
            if (prev.length === 0) {
              return prev;
            }
            const last = prev[prev.length - 1];
            if (last?.role === 'assistant' && !last.content) {
              return prev.slice(0, -2);
            }
            return prev;
          });
          controllerRef.current?.abort();
          controllerRef.current = null;
          bufferRef.current.delete(convId);
        },
      });
      controllerRef.current = controller;
    },
    [conversationId, updateBuffer, setBufferStreaming],
  );

  const stop = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    setStreaming(false);
    if (conversationId) {
      bufferRef.current.delete(conversationId);
      chatApi.stopGeneration(conversationId).catch(() => {});
    }
  }, [conversationId]);

  // 加载更早消息
  const loadMore = useCallback(async () => {
    if (!conversationId || !hasMore || loading) {
      return;
    }
    const firstId = serverMessages[0]?.id ?? 0;
    try {
      const res = await chatApi.listMessages(
        conversationId,
        firstId,
        PAGE_SIZE,
      );
      if (res.data.code === 0) {
        const older = res.data.data ?? [];
        setServerMessages((prev) => [...older, ...prev]);
        setHasMore(older.length >= PAGE_SIZE);
      }
    } catch {
      setError('加载更多失败');
    }
  }, [conversationId, hasMore, loading, serverMessages]);

  return {
    messages,
    loading,
    streaming,
    hasMore,
    error,
    send,
    stop,
    loadMore,
  };
}

export type { PendingMessage };
export type ChatHook = ReturnType<typeof useChat> & {
  messages: PendingMessage[];
  loading: boolean;
  streaming: boolean;
  hasMore: boolean;
  error: string | null;
  send: (content: string) => void;
  stop: () => void;
  loadMore: () => Promise<void>;
};
