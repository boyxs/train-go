'use client';

import {
  CloseOutlined,
  CommentOutlined,
  ExpandOutlined,
  HistoryOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { App, Button, Drawer } from 'antd';
import { Bot } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React, { useRef, useState } from 'react';

import { useChat } from '@/hooks/useChat';
import { useConversations } from '@/hooks/useConversations';
import { ChatInput } from '@/views/chat/ChatInput';
import { ChatMessages } from '@/views/chat/ChatMessages';
import { ChatSidebar } from '@/views/chat/ChatSidebar';

export const ChatBubble: React.FC = () => {
  const { message } = App.useApp();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const initializedRef = useRef(false);

  const {
    conversations,
    activeId,
    loading: historyLoading,
    fetchList,
    create,
    remove,
    select,
  } = useConversations();
  const { messages, loading, streaming, hasMore, send, stop, loadMore } =
    useChat(activeId);

  const handleOpen = async () => {
    setOpen(true);
    if (initializedRef.current) {
      return;
    }
    initializedRef.current = true;
    try {
      await fetchList();
    } catch {
      message.error('初始化对话失败');
    }
  };

  const handleCreate = async () => {
    try {
      const ok = await create();
      if (ok) {
        setShowHistory(false);
      }
    } catch {
      message.error('创建对话失败');
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await remove(id);
    } catch {
      message.error('删除失败');
    }
  };

  const handleSelect = (id: number) => {
    select(id);
    setShowHistory(false);
  };

  const handleShowHistory = async () => {
    setShowHistory(true);
    try {
      await fetchList();
    } catch {
      message.error('加载历史失败');
    }
  };

  return (
    <>
      {!open && (
        <button
          onClick={handleOpen}
          className='fixed bottom-6 right-6 z-50 w-12 h-12 rounded-full border-none cursor-pointer shadow-lg flex items-center justify-center transition-transform hover:scale-110'
          style={{ backgroundColor: '#0D9488' }}
        >
          <CommentOutlined style={{ fontSize: 22, color: '#fff' }} />
        </button>
      )}

      <Drawer
        title={
          <div className='flex items-center justify-between'>
            <div className='flex items-center gap-2'>
              <Bot size={16} color='#0D9488' />
              <span>AI 客服</span>
            </div>
            <div className='flex items-center gap-0.5'>
              <Button
                type='text'
                size='small'
                icon={<PlusOutlined />}
                onClick={handleCreate}
                title='新对话'
              />
              <Button
                type='text'
                size='small'
                icon={<HistoryOutlined />}
                onClick={
                  showHistory ? () => setShowHistory(false) : handleShowHistory
                }
                title={showHistory ? '返回' : '历史'}
              />
              <Button
                type='text'
                size='small'
                icon={<ExpandOutlined />}
                onClick={() => {
                  setOpen(false);
                  router.push('/chat');
                }}
                title='全屏'
              />
              <Button
                type='text'
                size='small'
                icon={<CloseOutlined />}
                onClick={() => {
                  setOpen(false);
                  setShowHistory(false);
                }}
              />
            </div>
          </div>
        }
        placement='right'
        closable={false}
        onClose={() => {
          setOpen(false);
          setShowHistory(false);
        }}
        open={open}
        width={380}
        styles={{
          body: { padding: 0, display: 'flex', flexDirection: 'column' },
        }}
      >
        {showHistory ? (
          <ChatSidebar
            conversations={conversations}
            activeId={activeId}
            onSelect={handleSelect}
            onCreate={handleCreate}
            onDelete={handleDelete}
            loading={historyLoading}
            hideCreateButton
          />
        ) : (
          <div className='flex flex-col h-full'>
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
        )}
      </Drawer>
    </>
  );
};
