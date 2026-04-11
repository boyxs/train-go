'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

import { ChatBubble } from '@/components/chat/ChatBubble';
import { AppLayout } from '@/components/layout/AppLayout';
import { AuthGuard } from '@/components/layout/AuthGuard';

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider
      theme={{
        token: { colorPrimary: '#0D9488' },
        components: {
          Menu: {
            itemSelectedColor: '#0D9488',
            horizontalItemSelectedColor: '#0D9488',
            itemHoverColor: '#0D9488',
          },
        },
      }}
    >
      <App>
        <AuthGuard>
          <AppLayout>{children}</AppLayout>
          <ChatBubble />
        </AuthGuard>
      </App>
    </ConfigProvider>
  );
}
