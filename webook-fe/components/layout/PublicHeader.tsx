'use client';

import { HomeOutlined, LoginOutlined, ReadOutlined } from '@ant-design/icons';
import { Button, Space } from 'antd';
import { useRouter } from 'next/navigation';
import React, { useSyncExternalStore } from 'react';

import { tokenUtil } from '@/utils/token';

function subscribe(cb: () => void) {
  window.addEventListener('storage', cb);
  return () => window.removeEventListener('storage', cb);
}

export const PublicHeader: React.FC = () => {
  const router = useRouter();
  const loggedIn = useSyncExternalStore(
    subscribe,
    () => tokenUtil.hasToken(),
    () => false,
  );

  return (
    <header className='shrink-0 flex items-center justify-between bg-white px-4 md:px-6 py-3 border-b border-gray-100'>
      <div
        className='flex items-center gap-2 cursor-pointer'
        onClick={() => router.push('/feed')}
      >
        <ReadOutlined className='text-blue-500' />
        <span className='text-base md:text-lg font-semibold'>小微书</span>
      </div>
      {loggedIn ? (
        <Button icon={<HomeOutlined />} onClick={() => router.push('/')}>
          返回首页
        </Button>
      ) : (
        <Space>
          <Button onClick={() => router.push('/register')}>注册</Button>
          <Button
            type='primary'
            icon={<LoginOutlined />}
            onClick={() => router.push('/login')}
          >
            登录
          </Button>
        </Space>
      )}
    </header>
  );
};
