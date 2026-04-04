'use client';

import {
  ArrowLeftOutlined,
  MenuOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { App, Button, Drawer } from 'antd';
import { useRouter } from 'next/navigation';
import React, { useEffect, useState } from 'react';

import { useChat } from '@/hooks/useChat';
import { useConversations } from '@/hooks/useConversations';

import { ChatInput } from './ChatInput';
import { ChatMessages } from './ChatMessages';
import { ChatSidebar } from './ChatSidebar';

function ChatPage() {
  const { message } = App.useApp();
  const router = useRouter();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const {
    conversations,
    activeId,
    loading: listLoading,
    fetchList,
    create,
    remove,
    select,
  } = useConversations();

  const { messages, loading, streaming, hasMore, error, send, stop, loadMore } =
    useChat(activeId);

  useEffect(() => {
    fetchList().catch(() => message.error('加载对话列表失败'));
  }, [fetchList, message]);

  useEffect(() => {
    if (error) {
      message.error(error);
    }
  }, [error, message]);

  const handleCreate = async () => {
    const ok = await create().catch(() => false);
    if (!ok) {
      message.error('创建对话失败');
    }
    setDrawerOpen(false);
  };

  const handleDelete = async (id: number) => {
    const ok = await remove(id).catch(() => false);
    if (!ok) {
      message.error('删除对话失败');
    }
  };

  const handleSelect = (id: number) => {
    select(id);
    setDrawerOpen(false);
  };

  const sidebar = (
    <ChatSidebar
      conversations={conversations}
      activeId={activeId}
      onSelect={handleSelect}
      onCreate={handleCreate}
      onDelete={handleDelete}
      loading={listLoading}
    />
  );

  return (
    <div className='flex flex-col h-full'>
      {/* 顶部栏 */}
      <div className='flex items-center justify-between px-4 md:px-6 h-14 bg-white border-b border-[#E5E7EB] shrink-0'>
        <div className='flex items-center gap-2'>
          {/* 移动端：返回按钮 */}
          <Button
            className='md:hidden'
            type='text'
            icon={<ArrowLeftOutlined />}
            onClick={() => router.back()}
          />
          {/* 桌面端：logo */}
          <span
            className='hidden md:inline text-lg font-semibold text-[#0D9488] cursor-pointer'
            onClick={() => router.push('/')}
          >
            小微书
          </span>
        </div>
        {/* 移动端居中标题 */}
        <div className='flex md:hidden items-center gap-1.5'>
          <RobotOutlined style={{ color: '#0D9488' }} />
          <span className='text-sm font-semibold text-[#1A1A1A]'>AI 客服</span>
        </div>
        <div className='flex items-center gap-3'>
          <Button
            className='hidden md:inline-flex'
            type='text'
            icon={<ArrowLeftOutlined />}
            onClick={() => router.push('/feed')}
          >
            返回广场
          </Button>
          {/* 移动端：汉堡菜单 */}
          <Button
            className='md:hidden'
            type='text'
            icon={<MenuOutlined />}
            onClick={() => setDrawerOpen(true)}
          />
          {/* 桌面端：头像 */}
          <div
            className='hidden md:flex w-8 h-8 rounded-full bg-[#0D9488] items-center justify-center text-white text-sm font-semibold cursor-pointer'
            onClick={() => router.push('/user/profile')}
          >
            U
          </div>
        </div>
      </div>

      {/* 主体 */}
      <div className='flex flex-1 overflow-hidden'>
        {/* 桌面端侧栏 */}
        <div className='hidden md:block w-[280px] border-r border-[#E5E7EB] bg-[#FAFAFA]'>
          {sidebar}
        </div>

        {/* 移动端 Drawer */}
        <Drawer
          title='对话历史'
          placement='left'
          onClose={() => setDrawerOpen(false)}
          open={drawerOpen}
          width={280}
          styles={{ body: { padding: 0 } }}
        >
          {sidebar}
        </Drawer>

        {/* 对话区 */}
        <div className='flex-1 flex flex-col min-w-0'>
          <ChatMessages
            messages={messages}
            loading={loading}
            streaming={streaming}
            hasMore={hasMore}
            onSend={send}
            onLoadMore={loadMore}
          />
          <ChatInput streaming={streaming} onSend={send} onStop={stop} />
        </div>
      </div>
    </div>
  );
}

export default ChatPage;
