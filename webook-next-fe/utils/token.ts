const ACCESS_KEY = 'access-token';
const REFRESH_KEY = 'refresh-token';

export const ACCESS_HEADER = 'x-access-token';
export const REFRESH_HEADER = 'x-refresh-token';

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
