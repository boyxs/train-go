'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

import { AuthGuard } from '@/components/layout/AuthGuard';

export default function ChatLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider theme={{ token: { colorPrimary: '#0D9488' } }}>
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
