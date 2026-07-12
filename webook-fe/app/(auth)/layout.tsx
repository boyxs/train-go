'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

import { ANTD_THEME } from '@/constants/theme';

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider theme={ANTD_THEME}>
      <App>{children}</App>
    </ConfigProvider>
  );
}
