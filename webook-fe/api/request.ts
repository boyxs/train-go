import axios, { InternalAxiosRequestConfig } from 'axios';

import { ACCESS_HEADER, REFRESH_HEADER, tokenUtil } from '@/utils/token';

interface RefreshConfig extends InternalAxiosRequestConfig {
  __isRefresh?: boolean;
}

// 单一 baseURL：开发环境走 Next.js dev server rewrites（next.config.ts 配置）
// 部署环境走 nginx 反代，两套拓扑前端代码完全一致
const instance = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL || '/api',
  withCredentials: true,
  timeout: 20_000, // > 服务端 HTTP 超时(15s)，收得到服务端错误而非前端先超时
});

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
  tokenUtil.clear();
  if (typeof window !== 'undefined') {
    window.location.href = '/login';
  }
};

async function refreshToken(): Promise<string> {
  const rt = tokenUtil.getRefresh();
  if (!rt) {
    throw new Error('no refresh token');
  }

  const response = await instance.get('/user/refresh_token', {
    headers: { Authorization: bearer(rt) },
    __isRefresh: true,
  } as RefreshConfig);

  const token = response.headers[ACCESS_HEADER];
  if (!token) {
    throw new Error('missing token in response');
  }

  tokenUtil.setAccess(token);
  return token;
}

// 请求拦截：附加 access token
instance.interceptors.request.use(
  (req: RefreshConfig) => {
    if (!req.__isRefresh) {
      req.headers.setAuthorization(bearer(tokenUtil.getAccess()), true);
    }
    return req;
  },
  (err) => Promise.reject(err),
);

// 响应拦截：存储 token + 401 自动刷新
instance.interceptors.response.use(
  (res) => {
    const accessToken = res.headers[ACCESS_HEADER];
    if (accessToken) {
      tokenUtil.setAccess(accessToken);
    }

    const rt = res.headers[REFRESH_HEADER];
    if (rt) {
      tokenUtil.setRefresh(rt);
    }

    return res;
  },
  async (err) => {
    const status: number | undefined = err?.response?.status;
    const config: RefreshConfig | undefined = err?.config;

    if (status !== 401) {
      return Promise.reject(err);
    }

    // 防止 refresh 请求本身触发 401 死循环
    if (config?.__isRefresh) {
      redirectLogin();
      return Promise.reject(err);
    }

    const reason: string | undefined = err?.response?.data?.reason;
    if (reason !== 'ACCESS_TOKEN_EXPIRED') {
      if (reason === 'TOKEN_INVALID') {
        redirectLogin();
      }
      return Promise.reject(err);
    }

    if (isRefreshing) {
      return new Promise<string>((resolve, reject) => {
        reqQueue.push({ resolve, reject });
      }).then((token) => {
        config?.headers.setAuthorization(bearer(token));
        return instance(config!);
      });
    }

    isRefreshing = true;
    try {
      const token = await refreshToken();
      resolveQueue(token);
      config?.headers.setAuthorization(bearer(token));
      return instance(config!);
    } catch (e) {
      rejectQueue(e);
      redirectLogin();
      return Promise.reject(e);
    } finally {
      isRefreshing = false;
    }
  },
);

export default instance;
