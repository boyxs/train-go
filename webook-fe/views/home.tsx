'use client';

import { BookOpen, Pencil } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React from 'react';

function HomePage() {
  const router = useRouter();

  return (
    <div className='flex flex-col items-center pt-28 md:pt-40'>
      <h1 className='text-[28px] font-bold text-[#1A1A1A] m-0'>
        欢迎来到小微书
      </h1>
      <p className='text-[15px] text-[#9CA3AF] mt-4 mb-8'>
        在这里记录你的想法，分享你的故事
      </p>
      <div className='flex items-center gap-3'>
        <button
          onClick={() => router.push('/article/edit')}
          className='flex items-center gap-2 rounded-lg border-none cursor-pointer text-white bg-[#0D9488] hover:bg-[#0B8178] transition-colors'
          style={{ padding: '8px 16px' }}
        >
          <Pencil size={16} />
          <span className='text-[15px] font-semibold'>写文章</span>
        </button>
        <button
          onClick={() => router.push('/feed')}
          className='flex items-center gap-2 rounded-lg border border-[#E5E7EB] cursor-pointer bg-white hover:bg-[#FAFAFA] transition-colors'
          style={{ padding: '8px 16px' }}
        >
          <BookOpen size={16} color='#6B7280' />
          <span className='text-[15px] font-medium text-[#6B7280]'>
            文章广场
          </span>
        </button>
      </div>
    </div>
  );
}

export default HomePage;
