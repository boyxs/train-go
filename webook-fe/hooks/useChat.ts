import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import * as chatApi from '@/api/chat';
import type { Message } from '@/types/chat';

const PAGE_SIZE = 10;

export function useChat(conversationId: number | null) {
  const [serverMessages, setServerMessages] = useState<Message[]>([]);
  const [pendingMessages, setPendingMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const tempIdRef = useRef(-1);

  // conversationId 变化时加载最新消息
  useEffect(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;

    // 立即清空旧消息，避免切换对话时看到上一个对话的内容
    setServerMessages([]);
    setPendingMessages([]);
    setStreaming(false);
    setError(null);

    if (!conversationId) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);

    chatApi
      .listMessages(conversationId, 0, PAGE_SIZE)
      .then((res) => {
        if (cancelled) {
          return;
        }
        if (res.data.code === 0) {
          const msgs = res.data.data ?? [];
          setServerMessages(msgs);
          setHasMore(msgs.length >= PAGE_SIZE);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError('加载消息失败');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [conversationId]);

  // 上滑加载更早的消息
  const loadingMoreRef = useRef(false);
  const loadMore = useCallback(async () => {
    if (!conversationId || !hasMore || loadingMoreRef.current) {
      return;
    }
    const oldest = serverMessages[0];
    if (!oldest) {
      return;
    }
    loadingMoreRef.current = true;
    try {
      const res = await chatApi.listMessages(
        conversationId,
        oldest.id,
        PAGE_SIZE,
      );
      if (res.data.code === 0) {
        const older = res.data.data ?? [];
        setHasMore(older.length >= PAGE_SIZE);
        // 去重：过滤掉已存在的 id
        setServerMessages((prev) => {
          const existingIds = new Set(prev.map((m) => m.id));
          const unique = older.filter((m) => !existingIds.has(m.id));
          return [...unique, ...prev];
        });
      }
    } catch {
      // 静默失败
    } finally {
      loadingMoreRef.current = false;
    }
  }, [conversationId, hasMore, serverMessages]);

  const messages = useMemo(
    () => [...serverMessages, ...pendingMessages],
    [serverMessages, pendingMessages],
  );

  // 发送消息
  const send = useCallback(
    (content: string) => {
      if (!conversationId || !content.trim()) {
        return;
      }

      const userMsg: Message = {
        id: tempIdRef.current--,
        conversationId,
        role: 'user',
        content: content.trim(),
        createdAt: Date.now(),
      };
      const aiMsg: Message = {
        id: tempIdRef.current--,
        conversationId,
        role: 'assistant',
        content: '',
        createdAt: Date.now(),
      };

      setPendingMessages((prev) => [...prev, userMsg, aiMsg]);
      setStreaming(true);
      setError(null);

      const controller = chatApi.sendMessageSSE(
        conversationId,
        content.trim(),
        {
          onDelta: (text) => {
            setPendingMessages((prev) => {
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
          onDone: (data) => {
            setStreaming(false);
            setPendingMessages((prev) => {
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
            controllerRef.current = null;
          },
          onError: (err) => {
            setStreaming(false);
            setError(err.msg || '生成失败');
            // AI 气泡无内容 → 移除本轮 pending（用户消息+空 AI 气泡）
            // AI 气泡有内容 → 保留已输出的部分
            setPendingMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last?.role === 'assistant' && !last.content) {
                return prev.slice(0, -2);
              }
              return prev;
            });
            // 确保 SSE 连接断开
            controllerRef.current?.abort();
            controllerRef.current = null;
          },
        },
      );
      controllerRef.current = controller;
    },
    [conversationId],
  );

  const stop = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    setStreaming(false);
    if (conversationId) {
      chatApi.stopGeneration(conversationId).catch(() => {});
    }
  }, [conversationId]);

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
