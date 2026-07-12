'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

import { AuthGuard } from '@/components/layout/AuthGuard';
import { ANTD_THEME } from '@/constants/theme';

export default function ChatLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider theme={ANTD_THEME}>
      <App>
        <AuthGuard>
          <div className='h-screen flex flex-col overflow-hidden'>
            {children}
          </div>
        </AuthGuard>
      </App>
    </ConfigProvider>
  );
}
