'use client';

import { SendOutlined, StopOutlined } from '@ant-design/icons';
import { Button, Input } from 'antd';
import React, { useState } from 'react';

const { TextArea } = Input;

interface ChatInputProps {
  streaming: boolean;
  onSend: (content: string) => void;
  onStop: () => void;
}

export const ChatInput: React.FC<ChatInputProps> = ({
  streaming,
  onSend,
  onStop,
}) => {
  const [value, setValue] = useState('');

  const handleSend = () => {
    const trimmed = value.trim();
    if (!trimmed || streaming) {
      return;
    }
    onSend(trimmed);
    setValue('');
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className='bg-white border-t border-[#E5E7EB] px-4 py-3'>
      <div className='flex items-center gap-2'>
        <div className='flex-1 bg-[#F9FAFB] rounded-xl border border-[#E5E7EB] px-3 py-1.5 focus-within:border-[#0D9488] focus-within:ring-1 focus-within:ring-[#0D9488]/20 transition-all'>
          <TextArea
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder='请输入您的问题...'
            autoSize={{ minRows: 1, maxRows: 4 }}
            disabled={streaming}
            variant='borderless'
            style={{
              padding: '4px 0',
              fontSize: 14,
              resize: 'none',
              backgroundColor: 'transparent',
            }}
          />
        </div>
        {streaming ? (
          <Button
            type='default'
            danger
            shape='circle'
            icon={<StopOutlined />}
            onClick={onStop}
            style={{ flexShrink: 0 }}
          />
        ) : (
          <Button
            type='primary'
            shape='circle'
            icon={<SendOutlined style={{ fontSize: 14 }} />}
            onClick={handleSend}
            disabled={!value.trim()}
            style={{ flexShrink: 0 }}
          />
        )}
      </div>
    </div>
  );
};
