'use client';

import '@ant-design/v5-patch-for-react-19';
import React from 'react';

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <>{children}</>;
}
