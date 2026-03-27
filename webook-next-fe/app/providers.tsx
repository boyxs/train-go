'use client';

import '@ant-design/v5-patch-for-react-19';
import { StyleProvider } from '@ant-design/cssinjs';
import React from 'react';

export const Providers: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  return <StyleProvider layer>{children}</StyleProvider>;
};
