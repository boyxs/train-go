'use client';

import { RobotOutlined, UserOutlined } from '@ant-design/icons';
import React from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

import type { Message } from '@/types/chat';

interface MessageBubbleProps {
  message: Message;
  streaming?: boolean;
}

/** 三点跳动加载指示器 */
const TypingDots: React.FC = () => (
  <span className='inline-flex items-center gap-1 h-5'>
    {[0, 1, 2].map((i) => (
      <span
        key={i}
        className='w-1.5 h-1.5 rounded-full bg-[#0D9488]'
        style={{
          animation: 'dotBounce 1.2s ease-in-out infinite',
          animationDelay: `${i * 0.15}s`,
        }}
      />
    ))}
  </span>
);

export const MessageBubble: React.FC<MessageBubbleProps> = ({
  message,
  streaming,
}) => {
  const isUser = message.role === 'user';
  const isEmpty = !message.content;

  return (
    <div
      className={`flex gap-2.5 mb-4 ${isUser ? 'flex-row-reverse' : 'flex-row'}`}
    >
      {/* 头像 */}
      <div
        className={`w-8 h-8 rounded-full flex items-center justify-center shrink-0 ${
          isUser
            ? 'bg-[#0D9488]'
            : 'bg-gradient-to-br from-[#0D9488] to-[#0F766E]'
        }`}
      >
        {isUser ? (
          <UserOutlined style={{ fontSize: 14, color: '#fff' }} />
        ) : (
          <RobotOutlined style={{ fontSize: 14, color: '#fff' }} />
        )}
      </div>

      {/* 气泡 */}
      <div className={`max-w-[80%] ${isUser ? 'items-end' : 'items-start'}`}>
        {isUser ? (
          <div className='bg-[#0D9488] text-white px-4 py-2.5 rounded-2xl rounded-tr-md text-[14px] leading-relaxed whitespace-pre-wrap break-words'>
            {message.content}
          </div>
        ) : (
          <div className='bg-white border border-[#E5E7EB] px-4 py-3 rounded-2xl rounded-tl-md shadow-sm'>
            {streaming && isEmpty ? (
              <TypingDots />
            ) : (
              <div className='prose prose-sm max-w-none text-[#1A1A1A] prose-headings:text-[#1A1A1A] prose-headings:font-semibold prose-headings:mt-3 prose-headings:mb-1.5 prose-p:my-1.5 prose-p:leading-relaxed prose-code:text-[#0D9488] prose-code:bg-[#F3F4F6] prose-code:px-1 prose-code:py-0.5 prose-code:rounded prose-code:text-[13px] prose-code:before:content-none prose-code:after:content-none prose-pre:bg-[#1E1E1E] prose-pre:text-[#D4D4D4] prose-pre:rounded-lg prose-pre:text-[13px] prose-a:text-[#0D9488] prose-a:no-underline hover:prose-a:underline prose-strong:text-[#1A1A1A] prose-li:my-0.5 prose-li:marker:text-[#9CA3AF] prose-ul:my-1.5 prose-ol:my-1.5 prose-blockquote:border-l-[#0D9488] prose-blockquote:text-[#6B7280] prose-blockquote:my-2'>
                <Markdown remarkPlugins={[remarkGfm]}>
                  {message.content || ''}
                </Markdown>
                {streaming && !isEmpty && <TypingDots />}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
