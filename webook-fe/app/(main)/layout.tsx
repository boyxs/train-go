'use client';

import '@ant-design/v5-patch-for-react-19';
import { App, ConfigProvider } from 'antd';
import React from 'react';

import { ChatBubble } from '@/components/chat/ChatBubble';
import { AppLayout } from '@/components/layout/AppLayout';
import { AuthGuard } from '@/components/layout/AuthGuard';
import { ANTD_THEME, PALETTE } from '@/constants/theme';

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <ConfigProvider
      theme={{
        ...ANTD_THEME,
        components: {
          ...ANTD_THEME.components,
          Menu: {
            itemSelectedColor: PALETTE.primary,
            horizontalItemSelectedColor: PALETTE.primary,
            itemHoverColor: PALETTE.primary,
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
