'use client';

import React from 'react';

import { AppLayout } from '@/components/layout/AppLayout';
import { AuthGuard } from '@/components/layout/AuthGuard';

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <AuthGuard>
      <AppLayout>{children}</AppLayout>
    </AuthGuard>
  );
}
