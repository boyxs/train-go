'use client';

import React, { useEffect } from 'react';

import * as userApi from '@/api/user';
import { Loading } from '@/components/common/Loading';

function LoginWechat() {
  useEffect(() => {
    userApi.findWechatAuthUrl().then((res) => {
      const url = res.data?.data;
      if (url) {
        window.location.href = url;
      }
    });
  }, []);

  return <Loading tip='正在跳转微信登录...' />;
}

export default LoginWechat;
