import type { AxiosResponse } from 'axios';
import { useEffect, useState } from 'react';

/**
 * 通用异步请求 hook
 * 适用于页面加载时自动发起的 GET 请求
 *
 * @param fetcher 返回 AxiosResponse 的函数（返回 null 则跳过请求）
 * @param deps 依赖数组，变化时重新请求
 */
export function useRequest<T>(
  fetcher: () => Promise<AxiosResponse<T> | null>,
  deps: unknown[] = [],
) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    fetcher()
      .then((res) => {
        if (!cancelled) {
          setData(res ? res.data : null);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err?.message || '请求失败');
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { data, loading, error };
}
