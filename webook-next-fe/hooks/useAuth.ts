import { useRouter } from 'next/navigation';
import { useCallback } from 'react';

import * as userApi from '@/api/user';
import { tokenUtil } from '@/utils/token';

export function useAuth() {
  const router = useRouter();

  const isLoggedIn = useCallback((): boolean => {
    return tokenUtil.hasToken();
  }, []);

  const logout = useCallback(async () => {
    try {
      await userApi.logout();
    } finally {
      tokenUtil.clear();
      router.replace('/login');
    }
  }, [router]);

  return { isLoggedIn, logout };
}
