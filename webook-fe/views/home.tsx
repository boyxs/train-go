'use client';

import { BookOpen, Pencil } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React from 'react';

import { PALETTE } from '@/constants/theme';

function HomePage() {
  const router = useRouter();

  return (
    <div className='flex flex-col items-center pt-28 md:pt-40'>
      <h1 className='text-[28px] font-bold text-ink m-0'>欢迎来到小微书</h1>
      <p className='text-[15px] text-subtle mt-4 mb-8'>
        在这里记录你的想法，分享你的故事
      </p>
      <div className='flex items-center gap-3'>
        <button
          onClick={() => router.push('/article/edit')}
          className='flex items-center gap-2 rounded-lg border-none cursor-pointer text-white bg-primary hover:bg-primary-dark transition-colors'
          style={{ padding: '8px 16px' }}
        >
          <Pencil size={16} />
          <span className='text-[15px] font-semibold'>写文章</span>
        </button>
        <button
          onClick={() => router.push('/feed')}
          className='flex items-center gap-2 rounded-lg border border-line cursor-pointer bg-white hover:bg-surface-hover transition-colors'
          style={{ padding: '8px 16px' }}
        >
          <BookOpen size={16} color={PALETTE.muted} />
          <span className='text-[15px] font-medium text-muted'>文章广场</span>
        </button>
      </div>
    </div>
  );
}

export default HomePage;
