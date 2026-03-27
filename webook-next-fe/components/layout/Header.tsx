'use client';

import {
  EditOutlined,
  FileTextOutlined,
  HomeOutlined,
  LogoutOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Dropdown, Menu } from 'antd';
import type { MenuProps } from 'antd';
import { usePathname, useRouter } from 'next/navigation';
import React from 'react';

import { useAuth } from '@/hooks/useAuth';

const menuItems: MenuProps['items'] = [
  { key: '/', icon: <HomeOutlined />, label: '首页' },
  { key: '/article/list', icon: <FileTextOutlined />, label: '我的文章' },
  { key: '/article/edit', icon: <EditOutlined />, label: '写文章' },
];

export const Header: React.FC = () => {
  const router = useRouter();
  const pathname = usePathname();
  const { logout } = useAuth();

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

  return (
    <header className="flex items-center bg-white px-6 border-b border-gray-100">
      <div
        className="text-lg font-semibold mr-8 cursor-pointer"
        onClick={() => router.push('/')}
      >
        小微书
      </div>
      <Menu
        mode="horizontal"
        selectedKeys={[pathname]}
        items={menuItems}
        onClick={({ key }) => router.push(key)}
        style={{ flex: 1, border: 'none' }}
      />
      <Dropdown menu={{ items: dropdownItems }} placement="bottomRight">
        <Avatar
          icon={<UserOutlined />}
          className="cursor-pointer"
          style={{ backgroundColor: '#1677ff' }}
        />
      </Dropdown>
    </header>
  );
};
