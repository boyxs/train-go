'use client';

import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { App, Button, Empty, List } from 'antd';
import dayjs from 'dayjs';
import React from 'react';

import { PALETTE } from '@/constants/theme';
import type { Conversation } from '@/types/chat';

interface ChatSidebarProps {
  conversations: Conversation[];
  activeId: number | null;
  onSelect: (id: number) => void;
  onCreate: () => void;
  onDelete: (id: number) => void;
  loading: boolean;
  hideCreateButton?: boolean;
}

export const ChatSidebar: React.FC<ChatSidebarProps> = ({
  conversations,
  activeId,
  onSelect,
  onCreate,
  onDelete,
  loading,
  hideCreateButton,
}) => {
  const { modal } = App.useApp();

  const handleDelete = (id: number, e: React.MouseEvent) => {
    e.stopPropagation();
    modal.confirm({
      title: '删除对话',
      content: '删除后无法恢复，确定删除吗？',
      okButtonProps: { danger: true },
      onOk: () => onDelete(id),
    });
  };

  return (
    <div className='flex flex-col h-full'>
      {!hideCreateButton && (
        <div className='p-3'>
          <Button
            type='primary'
            icon={<PlusOutlined />}
            onClick={onCreate}
            block
            style={{ borderRadius: 8 }}
          >
            新建对话
          </Button>
        </div>
      )}

      <div className='flex-1 overflow-auto'>
        {conversations.length === 0 && !loading ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description='暂无对话' />
        ) : (
          <List
            loading={loading}
            dataSource={conversations}
            renderItem={(conv) => (
              <div
                key={conv.id}
                onClick={() => onSelect(conv.id)}
                className={`mx-2 mb-1 px-3 py-2 rounded-lg cursor-pointer flex items-center justify-between group transition-colors ${
                  activeId === conv.id
                    ? 'bg-primary/10'
                    : 'hover:bg-surface-hover'
                }`}
              >
                <div className='min-w-0 flex-1'>
                  <div className='text-sm truncate text-ink'>
                    {conv.title || '新对话'}
                  </div>
                  <div className='text-xs text-subtle mt-0.5'>
                    {dayjs(conv.updatedAt).format('MM-DD HH:mm')}
                  </div>
                </div>
                <Button
                  type='text'
                  size='small'
                  icon={<DeleteOutlined />}
                  onClick={(e) => handleDelete(conv.id, e)}
                  className='opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity'
                  style={{ color: PALETTE.danger }}
                />
              </div>
            )}
          />
        )}
      </div>
    </div>
  );
};
