'use client';

import {
  EditOutlined,
  FileTextOutlined,
  HomeOutlined,
  LogoutOutlined,
  MenuOutlined,
  ReadOutlined,
  SearchOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Button, Drawer, Dropdown, Input, Menu } from 'antd';
import type { MenuProps } from 'antd';
import { Bot } from 'lucide-react';
import { usePathname, useRouter } from 'next/navigation';
import React, { useState } from 'react';

import { useAuth } from '@/hooks/useAuth';

const menuItems: MenuProps['items'] = [
  { key: '/', icon: <HomeOutlined />, label: '首页' },
  { key: '/feed', icon: <ReadOutlined />, label: '文章广场' },
  { key: '/article/list', icon: <FileTextOutlined />, label: '我的文章' },
  { key: '/article/edit', icon: <EditOutlined />, label: '写文章' },
  { key: '/search', icon: <SearchOutlined />, label: '搜索' },
  { key: '/chat', icon: <Bot size={14} />, label: 'AI 客服' },
];

export const Header: React.FC = () => {
  const router = useRouter();
  const pathname = usePathname();
  const { logout } = useAuth();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const navigate = (key: string) => {
    router.push(key);
    setDrawerOpen(false);
  };

  const dropdownItems: MenuProps['items'] = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人信息',
      onClick: () => router.push('/user/profile'),
    },
    { type: 'divider' },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      danger: true,
      onClick: logout,
    },
  ];

  const mobileMenuItems: MenuProps['items'] = [
    ...menuItems!,
    { type: 'divider' },
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人信息',
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      danger: true,
    },
  ];

  const onMobileMenuClick = ({ key }: { key: string }) => {
    if (key === 'logout') {
      logout();
    } else if (key === 'profile') {
      navigate('/user/profile');
    } else {
      navigate(key);
    }
  };

  return (
    <header className='flex items-center bg-white px-3 md:px-6 border-b border-gray-100'>
      <div
        className='text-base md:text-lg font-semibold mr-4 md:mr-8 cursor-pointer shrink-0'
        onClick={() => router.push('/')}
      >
        小微书
      </div>

      {/* 桌面端：水平菜单 + 搜索框 + 头像下拉 */}
      <div className='hidden md:flex flex-1 items-center'>
        <Menu
          mode='horizontal'
          selectedKeys={[pathname]}
          items={menuItems}
          onClick={({ key }) => router.push(key)}
          style={{ flex: 1, border: 'none' }}
        />
        <Input.Search
          placeholder='搜索文章...'
          size='small'
          style={{ width: 180, marginRight: 16 }}
          onSearch={(val) => {
            const q = val.trim();
            if (q) {
              router.push(`/search?q=${encodeURIComponent(q)}`);
            }
          }}
        />
        <Dropdown menu={{ items: dropdownItems }} placement='bottomRight'>
          <Avatar
            icon={<UserOutlined />}
            className='cursor-pointer'
            style={{ backgroundColor: '#0D9488' }}
          />
        </Dropdown>
      </div>

      {/* 移动端：汉堡按钮 */}
      <div className='flex md:hidden flex-1 justify-end'>
        <Button
          type='text'
          icon={<MenuOutlined />}
          onClick={() => setDrawerOpen(true)}
        />
      </div>

      {/* 移动端抽屉菜单 */}
      <Drawer
        title='小微书'
        placement='right'
        onClose={() => setDrawerOpen(false)}
        open={drawerOpen}
        width={260}
      >
        <Menu
          mode='vertical'
          selectedKeys={[pathname]}
          items={mobileMenuItems}
          onClick={onMobileMenuClick}
          style={{ border: 'none' }}
        />
      </Drawer>
    </header>
  );
};
