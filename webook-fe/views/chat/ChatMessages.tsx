'use client';

import { LoadingOutlined, RobotOutlined } from '@ant-design/icons';
import { Spin } from 'antd';
import React, { useCallback, useLayoutEffect, useRef } from 'react';

import type { Message } from '@/types/chat';

import { MessageBubble } from './MessageBubble';

interface ChatMessagesProps {
  messages: Message[];
  loading: boolean;
  streaming: boolean;
  hasMore?: boolean;
  onSend?: (content: string) => void;
  onLoadMore?: () => void;
}

export const ChatMessages: React.FC<ChatMessagesProps> = ({
  messages,
  loading,
  streaming,
  hasMore,
  onSend,
  onLoadMore,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const loadingMoreRef = useRef(false);
  const prevScrollHeightRef = useRef(0);
  const prevMsgLenRef = useRef(0);
  const initializedRef = useRef(false);

  // 标记 + 滚动合并在同一个 useLayoutEffect，
  // 确保在浏览器绘制前完成，用户看不到滚动跳动
  useLayoutEffect(() => {
    if (messages.length === 0) {
      initializedRef.current = false;
      prevMsgLenRef.current = 0;
      return;
    }

    const el = containerRef.current;
    if (!el) {
      return;
    }

    let action: 'bottom' | 'preserve' | null = null;

    if (loadingMoreRef.current) {
      action = 'preserve';
      loadingMoreRef.current = false;
    } else if (!initializedRef.current) {
      action = 'bottom';
      initializedRef.current = true;
    } else if (messages.length > prevMsgLenRef.current) {
      action = 'bottom';
    } else {
      // streaming：内容变化但长度不变，用户在底部附近则跟随
      const gap = el.scrollHeight - el.scrollTop - el.clientHeight;
      if (gap < 50) {
        action = 'bottom';
      }
    }

    prevMsgLenRef.current = messages.length;

    if (action === 'bottom') {
      el.scrollTop = el.scrollHeight;
    } else if (action === 'preserve') {
      el.scrollTop = el.scrollHeight - prevScrollHeightRef.current;
    }
  }, [messages]);

  const handleScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el || !hasMore || !onLoadMore || loadingMoreRef.current) {
      return;
    }
    if (el.scrollTop < 60) {
      loadingMoreRef.current = true;
      prevScrollHeightRef.current = el.scrollHeight;
      onLoadMore();
    }
  }, [hasMore, onLoadMore]);

  if (loading) {
    return (
      <div className='flex-1 flex items-center justify-center bg-[#FAFAFA]'>
        <Spin
          indicator={
            <LoadingOutlined style={{ fontSize: 24, color: '#0D9488' }} spin />
          }
        />
      </div>
    );
  }

  if (messages.length === 0) {
    return (
      <div className='flex-1 flex flex-col items-center justify-end pb-6 px-5 bg-[#FAFAFA]'>
        <div className='w-12 h-12 rounded-2xl bg-gradient-to-br from-[#0D9488] to-[#0F766E] flex items-center justify-center shadow-sm mb-3'>
          <RobotOutlined style={{ fontSize: 22, color: '#fff' }} />
        </div>
        <div className='text-[15px] font-semibold text-[#1A1A1A] mb-1'>
          小微书 AI 客服
        </div>
        <div className='text-xs text-[#9CA3AF] mb-5'>
          随时为你解答平台问题、推荐优质文章
        </div>
        <div className='flex flex-col gap-2 w-full max-w-xs'>
          {[
            { icon: '📝', text: '怎么发布文章？' },
            { icon: '🔥', text: '推荐热门文章' },
            { icon: '❓', text: '平台有什么功能？' },
          ].map((q) => (
            <button
              key={q.text}
              type='button'
              onClick={() => onSend?.(q.text)}
              className='flex items-center gap-2.5 w-full px-4 py-3 bg-white rounded-xl border border-[#E5E7EB] cursor-pointer hover:border-[#0D9488] hover:bg-[#0D9488]/[0.02] active:scale-[0.98] transition-all text-left'
            >
              <span>{q.icon}</span>
              <span className='text-sm text-[#1A1A1A]'>{q.text}</span>
            </button>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className='flex-1 overflow-y-auto bg-[#FAFAFA] px-4 py-4'
    >
      {hasMore && (
        <div className='text-center py-3 text-xs text-[#9CA3AF]'>
          <LoadingOutlined className='mr-1' />
          上滑加载更早消息
        </div>
      )}
      {!hasMore && messages.length > 0 && (
        <div className='text-center py-3 text-xs text-[#9CA3AF]'>
          已是最早消息
        </div>
      )}
      {messages.map((msg, idx) => (
        <MessageBubble
          key={msg.id}
          message={msg}
          streaming={
            streaming && msg.role === 'assistant' && idx === messages.length - 1
          }
        />
      ))}
    </div>
  );
};
