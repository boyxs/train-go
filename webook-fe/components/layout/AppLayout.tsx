'use client';

import React from 'react';

import { Header } from './Header';

interface AppLayoutProps {
  children: React.ReactNode;
}

export const AppLayout: React.FC<AppLayoutProps> = ({ children }) => {
  return (
    <div className='h-screen flex flex-col overflow-hidden bg-page'>
      <Header />
      <main className='flex-1 overflow-y-auto px-3 py-3 md:px-6 md:py-4'>
        {children}
      </main>
      <footer className='text-center text-gray-400 py-2 text-sm'>
        小微书 &copy; {new Date().getFullYear()}
      </footer>
    </div>
  );
};
