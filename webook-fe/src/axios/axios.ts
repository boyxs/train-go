import axios, { InternalAxiosRequestConfig } from 'axios';

interface RefreshConfig extends InternalAxiosRequestConfig {
  __isRefresh?: boolean;
}

const axiosInstance = axios.create({
  baseURL: 'http://localhost:8089',
  // baseURL: 'http://192.168.150.101:8089',
  withCredentials: true,
  timeout: 10_000,
});

const ACCESS_KEY = 'access-token';
const REFRESH_KEY = 'refresh-token';
const ACCESS_HEADER = 'x-access-token';
const REFRESH_HEADER = 'x-refresh-token';

const bearer = (token: string | null) => (token ? `Bearer ${token}` : '');

let isRefreshing = false;
let reqQueue: Array<{
  resolve: (t: string) => void;
  reject: (e: unknown) => void;
}> = [];

const resolveQueue = (token: string) => {
  reqQueue.forEach(({ resolve }) => resolve(token));
  reqQueue = [];
};

const rejectQueue = (error: unknown) => {
  reqQueue.forEach(({ reject }) => reject(error));
  reqQueue = [];
};

const redirectLogin = () => {
  localStorage.removeItem(ACCESS_KEY);
  localStorage.removeItem(REFRESH_KEY);
  window.location.href = '/user/login';
};

async function refreshToken(): Promise<string> {
  const _refreshToken = localStorage.getItem(REFRESH_KEY);
  if (!_refreshToken) {
    throw new Error('no refresh token');
  }

  const response = await axiosInstance.get('/user/refresh_token', {
    headers: { Authorization: bearer(_refreshToken) },
    __isRefresh: true,
  } as RefreshConfig);

  const token = response.headers[ACCESS_HEADER];
  if (!token) {
    throw new Error('missing token in response');
  }

  localStorage.setItem(ACCESS_KEY, token);
  return token;
}

axiosInstance.interceptors.request.use(
  (req: RefreshConfig) => {
    if (!req.__isRefresh) {
      const accessToken = localStorage.getItem(ACCESS_KEY);
      req.headers.setAuthorization(bearer(accessToken), true);
    }
    return req;
  },
  (err) => {
    console.error(err);
    return Promise.reject(err);
  },
);

axiosInstance.interceptors.response.use(
  (res) => {
    const accessToken = res.headers[ACCESS_HEADER];
    if (accessToken) {
      localStorage.setItem(ACCESS_KEY, accessToken);
    }

    const refreshToken = res.headers[REFRESH_HEADER];
    if (refreshToken) {
      localStorage.setItem(REFRESH_KEY, refreshToken);
    }

    return res;
  },
  async (err) => {
    const status: number | undefined = err?.response?.status;
    const config: RefreshConfig | undefined = err?.config;

    if (status !== 401) {
      return Promise.reject(err);
    }

    //防止refresh请求本身触发401死循环
    if (config?.__isRefresh) {
      redirectLogin();
      return Promise.reject(err);
    }

    if (isRefreshing) {
      return new Promise<string>((resolve, reject) => {
        //回调存进队列
        reqQueue.push({ resolve, reject });
      }).then((token) => {
        config?.headers.setAuthorization(bearer(token));
        return axiosInstance(config!);
      });
    }

    isRefreshing = true;
    try {
      const token = await refreshToken();
      resolveQueue(token);
      config?.headers.setAuthorization(bearer(token));
      return axiosInstance(config!);
    } catch (e) {
      rejectQueue(e);
      redirectLogin();
      return Promise.reject(e);
    } finally {
      isRefreshing = false;
    }
  },
);

export default axiosInstance;
