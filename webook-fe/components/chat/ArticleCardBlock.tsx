'use client';

import { FileTextOutlined } from '@ant-design/icons';
import { useRouter } from 'next/navigation';
import React from 'react';

import { recordAIClick } from '@/api/ai';
import type { ArticleCard } from '@/types/chat';

interface ArticleCardBlockProps {
  articles: ArticleCard[];
  conversationId?: number;
}

export const ArticleCardBlock: React.FC<ArticleCardBlockProps> = ({
  articles,
  conversationId,
}) => {
  const router = useRouter();

  const handleClick = (articleId: number) => {
    if (conversationId) {
      recordAIClick({
        article_id: articleId,
        conversation_id: conversationId,
      }).catch(() => {});
    }
    router.push(`/article/${articleId}`);
  };

  if (articles.length === 0) {
    return <div className='text-xs text-[#9CA3AF] py-1'>暂无相关文章</div>;
  }

  return (
    <div className='flex flex-col gap-2 my-2'>
      {articles.map((article) => (
        <button
          key={article.id}
          type='button'
          onClick={() => handleClick(article.id)}
          className='flex items-start gap-2.5 w-full p-3 bg-[#F9FAFB] rounded-lg border border-[#E5E7EB] text-left cursor-pointer hover:border-[#0D9488] hover:bg-[#0D9488]/[0.02] active:scale-[0.99] transition-all'
        >
          <FileTextOutlined
            style={{
              fontSize: 14,
              color: '#0D9488',
              marginTop: 2,
              flexShrink: 0,
            }}
          />
          <div className='min-w-0'>
            <div className='text-[13px] font-semibold text-[#1A1A1A] truncate'>
              {article.title}
            </div>
            {article.abstract && (
              <div className='text-[12px] text-[#6B7280] mt-0.5 line-clamp-2 leading-relaxed'>
                {article.abstract}
              </div>
            )}
          </div>
        </button>
      ))}
    </div>
  );
};
