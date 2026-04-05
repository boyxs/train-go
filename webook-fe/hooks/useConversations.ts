import { useCallback, useState } from 'react';

import * as chatApi from '@/api/chat';
import type { Conversation } from '@/types/chat';

/**
 * 管理对话列表的 CRUD 逻辑
 * 被 ChatBubble 和 ChatPage 共享
 */
export function useConversations() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeId, setActiveId] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);

  // 加载列表，首次自动选中第一个或新建
  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const res = await chatApi.listConversations();
      if (res.data.code !== 0) {
        return;
      }
      const list = res.data.data ?? [];
      setConversations(list);

      if (list.length > 0) {
        setActiveId((prev) => prev ?? list[0].id);
      } else {
        // 无对话自动创建
        const createRes = await chatApi.createConversation();
        if (createRes.data.code === 0) {
          const conv = createRes.data.data;
          setConversations([conv]);
          setActiveId(conv.id);
        }
      }
    } finally {
      setLoading(false);
    }
  }, []);

  // 新建对话
  const create = useCallback(async (): Promise<boolean> => {
    const res = await chatApi.createConversation();
    if (res.data.code === 0) {
      const conv = res.data.data;
      setConversations((prev) => [conv, ...prev]);
      setActiveId(conv.id);
      return true;
    }
    return false;
  }, []);

  // 删除对话
  const remove = useCallback(async (id: number): Promise<boolean> => {
    const res = await chatApi.deleteConversation(id);
    if (res.data.code === 0) {
      setConversations((prev) => prev.filter((c) => c.id !== id));
      setActiveId((prev) => (prev === id ? null : prev));
      return true;
    }
    return false;
  }, []);

  // 选中对话
  const select = useCallback((id: number) => {
    setActiveId(id);
  }, []);

  return {
    conversations,
    activeId,
    loading,
    fetchList,
    create,
    remove,
    select,
  };
}
