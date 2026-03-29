import { STORAGE_KEYS } from '@/constants/storage';

export { ACCESS_HEADER, REFRESH_HEADER } from '@/constants/http';

const ACCESS_KEY = STORAGE_KEYS.ACCESS_TOKEN;
const REFRESH_KEY = STORAGE_KEYS.REFRESH_TOKEN;

export const tokenUtil = {
  getAccess: () => localStorage.getItem(ACCESS_KEY),
  setAccess: (token: string) => localStorage.setItem(ACCESS_KEY, token),
  getRefresh: () => localStorage.getItem(REFRESH_KEY),
  setRefresh: (token: string) => localStorage.setItem(REFRESH_KEY, token),
  clear: () => {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  },
  hasToken: () => !!localStorage.getItem(ACCESS_KEY),
};
