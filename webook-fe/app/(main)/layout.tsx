'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

import { AppLayout } from '@/components/layout/AppLayout';
import { AuthGuard } from '@/components/layout/AuthGuard';

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider theme={{ token: { colorPrimary: '#0D9488' } }}>
      <App>
        <AuthGuard>
          <AppLayout>{children}</AppLayout>
        </AuthGuard>
      </App>
    </ConfigProvider>
  );
}
