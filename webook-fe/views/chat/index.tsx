'use client';

import { ArrowLeftOutlined, MenuOutlined } from '@ant-design/icons';
import { App, Button, Drawer } from 'antd';
import { Bot } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React, { useEffect, useState } from 'react';

import { PALETTE } from '@/constants/theme';
import { useChat } from '@/hooks/useChat';
import { useConversations } from '@/hooks/useConversations';
import { getErrorMessage } from '@/utils/apiError';

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

  const {
    messages,
    loading,
    streaming,
    hasMore,
    error,
    send,
    stop,
    loadMore,
    setFeedback,
  } = useChat(activeId);

  useEffect(() => {
    fetchList().catch((e) =>
      message.error(getErrorMessage(e, '加载对话列表失败')),
    );
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

  // 发送消息——如果没有活跃对话，先创建一个
  const handleSend = async (content: string) => {
    if (!activeId) {
      const newId = await create().catch(() => -1);
      if (newId <= 0) {
        message.error('创建对话失败');
        return;
      }
      send(content, newId);
      return;
    }
    send(content);
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
      <div className='flex items-center justify-between px-4 md:px-6 h-14 bg-white border-b border-line shrink-0'>
        {/* 左侧 */}
        <div className='flex items-center gap-2'>
          {/* 移动端：返回广场 */}
          <div className='md:hidden'>
            <Button
              type='text'
              icon={<ArrowLeftOutlined />}
              onClick={() => router.push('/feed')}
            />
          </div>
          {/* 桌面端：logo 点击回首页 */}
          <span
            className='hidden md:inline text-lg font-semibold text-primary cursor-pointer'
            onClick={() => router.push('/')}
          >
            小微书
          </span>
        </div>

        {/* 中间标题（移动端） */}
        <div className='flex md:hidden items-center gap-1.5'>
          <Bot size={16} color={PALETTE.primary} />
          <span className='text-sm font-semibold text-ink'>AI 客服</span>
        </div>

        {/* 右侧 */}
        <div className='flex items-center gap-3'>
          {/* 桌面端：文章广场 + 我的文章 + 头像 */}
          <div className='hidden md:flex items-center gap-2'>
            <Button type='text' onClick={() => router.push('/feed')}>
              文章广场
            </Button>
            <Button type='text' onClick={() => router.push('/article/list')}>
              我的文章
            </Button>
            <div
              className='w-8 h-8 rounded-full bg-primary flex items-center justify-center text-white text-sm font-semibold cursor-pointer'
              onClick={() => router.push('/user/profile')}
            >
              U
            </div>
          </div>
          {/* 移动端：汉堡菜单 */}
          <div className='md:hidden'>
            <Button
              type='text'
              icon={<MenuOutlined />}
              onClick={() => setDrawerOpen(true)}
            />
          </div>
        </div>
      </div>

      {/* 主体 */}
      <div className='flex flex-1 overflow-hidden'>
        {/* 桌面端侧栏 */}
        <div className='hidden md:block w-[280px] border-r border-line bg-surface-hover'>
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
            conversationId={activeId ?? undefined}
            onSend={handleSend}
            onLoadMore={loadMore}
            onFeedback={setFeedback}
          />
          <ChatInput streaming={streaming} onSend={handleSend} onStop={stop} />
        </div>
      </div>
    </div>
  );
}

export default ChatPage;
