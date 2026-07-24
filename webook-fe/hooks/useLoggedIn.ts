import { useSyncExternalStore } from 'react';

import { tokenUtil } from '@/utils/token';

// storage 事件订阅：跨标签登录/登出即时同步；SSR 无 window 时返回空退订。
function subscribe(cb: () => void) {
  if (typeof window === 'undefined') {
    return () => {};
  }
  window.addEventListener('storage', cb);
  return () => window.removeEventListener('storage', cb);
}

// 空订阅：仅用于「是否已在客户端 hydrate」判断（不监听任何外部源）。
const noopSubscribe = () => () => {};

// useLoggedIn SSR 安全的登录态：
//   - loggedIn：server / hydrate 前恒为 false（getServerSnapshot 不碰 localStorage，规避「localStorage is not defined」），
//     hydrate 后读真实 token；跨标签登录/登出经 storage 事件同步。
//   - hydrated：server=false、客户端 hydrate 后=true，供调用方在 hydrate 前渲染占位/加载态、
//     避免已登录用户闪现「未登录引导」。
export function useLoggedIn(): { loggedIn: boolean; hydrated: boolean } {
  const loggedIn = useSyncExternalStore(
    subscribe,
    () => tokenUtil.hasToken(),
    () => false,
  );
  const hydrated = useSyncExternalStore(
    noopSubscribe,
    () => true,
    () => false,
  );
  return { loggedIn, hydrated };
}
