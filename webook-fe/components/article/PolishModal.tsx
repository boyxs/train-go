'use client';

import { Modal } from 'antd';
import React from 'react';

import type { PolishResult } from '@/api/article';

interface PolishModalProps {
  open: boolean;
  original: { title: string; abstract: string; content: string };
  polished: PolishResult | null;
  onAccept: () => void;
  onCancel: () => void;
}

export const PolishModal: React.FC<PolishModalProps> = ({
  open,
  original,
  polished,
  onAccept,
  onCancel,
}) => {
  if (!polished) {
    return null;
  }

  return (
    <Modal
      open={open}
      title='AI 润色对比'
      width={900}
      onCancel={onCancel}
      okText='采纳'
      cancelText='放弃'
      onOk={onAccept}
    >
      <div className='flex gap-4 max-h-[60vh] overflow-auto'>
        {/* 左栏：原文 */}
        <div className='flex-1 min-w-0'>
          <div className='text-xs font-semibold text-[#9CA3AF] mb-2'>原文</div>
          <div className='bg-[#F5F5F5] rounded-lg p-4 space-y-3'>
            <div>
              <div className='text-xs text-[#9CA3AF] mb-1'>标题</div>
              <div className='text-sm text-[#1A1A1A] font-medium'>
                {original.title || '（空）'}
              </div>
            </div>
            <div>
              <div className='text-xs text-[#9CA3AF] mb-1'>摘要</div>
              <div className='text-sm text-[#6B7280]'>
                {original.abstract || '（空）'}
              </div>
            </div>
            <div>
              <div className='text-xs text-[#9CA3AF] mb-1'>正文</div>
              <div className='text-sm text-[#1A1A1A] whitespace-pre-wrap leading-relaxed'>
                {original.content || '（空）'}
              </div>
            </div>
          </div>
        </div>

        {/* 右栏：润色结果 */}
        <div className='flex-1 min-w-0'>
          <div className='text-xs font-semibold text-[#0D9488] mb-2'>
            润色结果
          </div>
          <div className='bg-[#F0FDFA] rounded-lg p-4 space-y-3 border border-[#0D9488]/20'>
            <div>
              <div className='text-xs text-[#0D9488] mb-1'>标题</div>
              <div className='text-sm text-[#1A1A1A] font-medium'>
                {polished.title}
              </div>
            </div>
            <div>
              <div className='text-xs text-[#0D9488] mb-1'>摘要</div>
              <div className='text-sm text-[#6B7280]'>{polished.abstract}</div>
            </div>
            <div>
              <div className='text-xs text-[#0D9488] mb-1'>正文</div>
              <div className='text-sm text-[#1A1A1A] whitespace-pre-wrap leading-relaxed'>
                {polished.content}
              </div>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
};
