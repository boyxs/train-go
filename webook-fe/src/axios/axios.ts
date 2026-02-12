import axios from 'axios';
const instance = axios.create({
  // 这边记得修改你对应的配置文件
  baseURL: 'http://localhost:8089',
  withCredentials: true,
});

const accessToken = 'access-token';

instance.interceptors.response.use(
  function (resp) {
    // const newToken = resp.headers['x-jwt-token'];
    // const newRefreshToken = resp.headers['x-refresh-token'];
    // console.log('resp headers', resp.headers);
    // if (newToken) {
    //     localStorage.setItem('token', newToken);
    // }
    // if (newRefreshToken) {
    //     localStorage.setItem('refresh_token', newRefreshToken);
    // }
    const token = resp?.headers?.['x-jwt-token'];
    if (token) {
      localStorage.setItem(accessToken, token);
    }
    if (resp?.status === 401) {
      localStorage.removeItem(accessToken);
      setTimeout(() => (window.location.href = '/user/login'), 3000);
    }
    return resp;
  },
  (err) => {
    console.log(err);
    if (err?.response?.status === 401) {
      localStorage.removeItem(accessToken);
      setTimeout(() => (window.location.href = '/user/login'), 3000);
    }
    return err;
  },
);

// 在这里让每一个请求都加上 authorization 的头部
instance.interceptors.request.use(
  (req) => {
    const token = localStorage.getItem(accessToken);
    req.headers.setAuthorization('Bearer ' + token, true);
    return req;
  },
  (err) => {
    console.log(err);
  },
);

export default instance;
