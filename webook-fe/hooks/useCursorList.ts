import { useCallback, useEffect, useRef, useState } from 'react';

import type { CursorList } from '@/types';
import { getErrorMessage } from '@/utils/apiError';

// 后端 relation 列表默认 limit=20（normLimit）；拉满一页才可能有更多
export const CURSOR_LIMIT = 20;

/**
 * 游标列表通用 hook（关注/粉丝/黑名单等 {list, nextCursor} 接口共用）
 * @param fetcher 传入 cursor（首页 0）返回一页 CursorList；deps 变化时重载
 */
export function useCursorList<T>(
  fetcher: (cursor: number, limit: number) => Promise<CursorList<T>>,
  deps: unknown[] = [],
) {
  const [items, setItems] = useState<T[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cursorRef = useRef(0);

  // deps 由调用方声明列表变化时机（同 useRequest 约定）
  const fetchPage = useCallback(
    (cursor: number) => fetcher(cursor, CURSOR_LIMIT),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    deps,
  );

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    cursorRef.current = 0;
    try {
      const { list, nextCursor } = await fetchPage(0);
      setItems(list ?? []);
      cursorRef.current = nextCursor ?? 0;
      setHasMore((list?.length ?? 0) >= CURSOR_LIMIT);
    } catch (e) {
      setError(getErrorMessage(e, '加载失败'));
    } finally {
      setLoading(false);
    }
  }, [fetchPage]);

  const loadMore = useCallback(async () => {
    if (loadingMore || !hasMore) {
      return;
    }
    setLoadingMore(true);
    setError(null);
    try {
      const { list, nextCursor } = await fetchPage(cursorRef.current);
      setItems((prev) => [...prev, ...(list ?? [])]);
      cursorRef.current = nextCursor ?? 0;
      setHasMore((list?.length ?? 0) >= CURSOR_LIMIT);
    } catch (e) {
      setError(getErrorMessage(e, '加载失败'));
    } finally {
      setLoadingMore(false);
    }
  }, [fetchPage, hasMore, loadingMore]);

  useEffect(() => {
    reload();
  }, [reload]);

  return {
    items,
    setItems,
    hasMore,
    loading,
    loadingMore,
    error,
    loadMore,
    reload,
  };
}
