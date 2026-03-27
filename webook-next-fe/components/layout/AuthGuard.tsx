'use client';

import { usePathname, useRouter } from 'next/navigation';
import React, { useEffect, useRef, useSyncExternalStore } from 'react';

import { Loading } from '@/components/common/Loading';
import { tokenUtil } from '@/utils/token';

interface AuthGuardProps {
  children: React.ReactNode;
}

function subscribeToToken(callback: () => void) {
  window.addEventListener('storage', callback);
  return () => window.removeEventListener('storage', callback);
}

function getTokenSnapshot() {
  return tokenUtil.hasToken();
}

function getServerSnapshot() {
  return false;
}

export const AuthGuard: React.FC<AuthGuardProps> = ({ children }) => {
  const router = useRouter();
  const pathname = usePathname();
  const hasToken = useSyncExternalStore(
    subscribeToToken,
    getTokenSnapshot,
    getServerSnapshot,
  );

  const hydrated = useRef(false);

  useEffect(() => {
    hydrated.current = true;
  }, []);

  useEffect(() => {
    if (hydrated.current && !tokenUtil.hasToken()) {
      router.replace(`/login?redirect=${encodeURIComponent(pathname)}`);
    }
  }, [hasToken, router, pathname]);

  if (!hasToken) {
    return <Loading />;
  }

  return <>{children}</>;
};
