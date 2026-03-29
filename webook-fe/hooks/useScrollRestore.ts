import { useCallback, useEffect, useRef } from 'react';

/**
 * 滚动位置记忆 hook
 *
 * 使用方式：
 * 1. ref 绑定到可滚动容器
 * 2. 在离开当前页面前调用 set()（如点击列表项时）
 * 3. 返回页面后自动恢复，恢复后自动清除（一次性消费）
 * 4. 翻页等场景调用 reset() 主动清除
 *
 * @param key sessionStorage key
 * @param ready 数据是否就绪（就绪后才恢复）
 */
export function useScrollRestore(key: string, ready: boolean) {
  const ref = useRef<HTMLDivElement>(null);

  const set = useCallback(() => {
    if (ref.current) {
      sessionStorage.setItem(key, String(ref.current.scrollTop));
    }
  }, [key]);

  // 数据就绪后恢复，恢复后立即清除（一次性消费）
  useEffect(() => {
    if (ready && ref.current) {
      const pos = sessionStorage.getItem(key);
      if (pos) {
        ref.current.scrollTop = Number(pos);
        sessionStorage.removeItem(key);
      }
    }
  }, [ready, key]);

  const reset = useCallback(() => {
    sessionStorage.removeItem(key);
  }, [key]);

  return [ref, set, reset] as const;
}
