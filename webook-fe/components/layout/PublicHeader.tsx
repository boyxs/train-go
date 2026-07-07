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

// 空订阅：仅用于「是否已在客户端 hydrate」判断（不监听任何外部源）
const noopSubscribe = () => () => {};

export const PublicHeader: React.FC = () => {
  const router = useRouter();
  const loggedIn = useSyncExternalStore(
    subscribe,
    () => tokenUtil.hasToken(),
    () => false,
  );
  // localStorage 只在客户端可读：hydrate 前不渲染登录态按钮，避免已登录用户闪现「注册/登录」
  // 用 useSyncExternalStore（server/首帧=false，hydrate 后=true）而非 setState-in-effect
  const hydrated = useSyncExternalStore(
    noopSubscribe,
    () => true,
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
      {/* hydrate 前占位（保留右侧高度、不渲染任何登录态按钮），hydrate 后再按真实登录态渲染 */}
      {!hydrated ? (
        <div aria-hidden style={{ height: 32 }} />
      ) : loggedIn ? (
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
