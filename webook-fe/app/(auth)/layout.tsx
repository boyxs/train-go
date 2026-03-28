'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider theme={{ token: { colorPrimary: '#0D9488' } }}>
      <App>{children}</App>
    </ConfigProvider>
  );
}
