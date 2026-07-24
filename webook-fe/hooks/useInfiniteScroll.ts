import { useEffect, useRef } from 'react';

// 提前触发距离：哨兵进入视口前 200px 即预加载，减少用户等待。
const ROOT_MARGIN = '200px';

/**
 * 无限滚动哨兵 hook：把返回的 ref 挂到列表底部的哨兵元素上，
 * 当它进入视口且 enabled=true 时调用 onReach（触底加载下一页）。
 * enabled 由调用方门控（如 !loading && !loadingMore && hasMore）防重复触发；
 * onReach 用 ref 存最新引用，避免其变化导致 observer 反复重建。≥2 个游标列表可复用（关注流 / 未来评论流）。
 */
export function useInfiniteScroll<T extends HTMLElement = HTMLDivElement>(
  onReach: () => void,
  enabled: boolean,
) {
  const sentinelRef = useRef<T | null>(null);
  const onReachRef = useRef(onReach);

  // 在 effect 中同步最新回调（禁止 render 期写 ref）；观察器 effect 只依赖 enabled，不因回调变化反复重建。
  useEffect(() => {
    onReachRef.current = onReach;
  }, [onReach]);

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !enabled) {
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          onReachRef.current();
        }
      },
      { rootMargin: ROOT_MARGIN },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [enabled]);

  return sentinelRef;
}
