'use client';

import { Button } from 'antd';
import React, { useState } from 'react';

import { PALETTE } from '@/constants/theme';

const MAX_LEN = 500;

interface CommentEditorProps {
  placeholder?: string;
  submitText?: string;
  replyTo?: string; // 回复目标昵称：设置后顶部显示「回复 @X」标题（对齐原型）
  autoFocus?: boolean;
  onSubmit: (content: string) => Promise<boolean>; // 返回 true 表示成功，清空输入
  onCancel?: () => void;
}

// CommentEditor 评论/回复输入框，对齐原型 输入框：bordered 容器 + 字数统计 + 发布按钮
export function CommentEditor({
  placeholder = '写下你的评论…',
  submitText = '发布',
  replyTo,
  autoFocus,
  onSubmit,
  onCancel,
}: CommentEditorProps) {
  const [content, setContent] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const trimmed = content.trim();
  const canSubmit =
    trimmed.length > 0 && trimmed.length <= MAX_LEN && !submitting;

  const handleSubmit = async () => {
    if (!canSubmit) {
      return;
    }
    setSubmitting(true);
    try {
      const ok = await onSubmit(trimmed);
      if (ok) {
        setContent('');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className='rounded-lg flex flex-col gap-2.5'
      style={{ border: `1px solid ${PALETTE.line}`, padding: '12px 14px' }}
    >
      {replyTo && (
        <span style={{ fontSize: 13, fontWeight: 600, color: PALETTE.primary }}>
          回复 @{replyTo}
        </span>
      )}
      <textarea
        autoFocus={autoFocus}
        value={content}
        maxLength={MAX_LEN}
        placeholder={replyTo ? '说点什么…' : placeholder}
        rows={2}
        onChange={(e) => setContent(e.target.value)}
        className='w-full resize-none border-none outline-none bg-transparent'
        style={{ fontSize: 14, color: PALETTE.ink, lineHeight: 1.6 }}
      />
      <div className='flex items-center justify-between'>
        <span style={{ fontSize: 12, color: PALETTE.subtle }}>
          {trimmed.length} / {MAX_LEN}
        </span>
        <div className='flex items-center gap-2'>
          {onCancel && (
            <Button size='small' onClick={onCancel}>
              取消
            </Button>
          )}
          <Button
            type='primary'
            size='small'
            loading={submitting}
            disabled={!canSubmit}
            onClick={handleSubmit}
          >
            {submitText}
          </Button>
        </div>
      </div>
    </div>
  );
}
