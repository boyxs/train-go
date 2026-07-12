'use client';

import { FileTextOutlined } from '@ant-design/icons';
import { useRouter } from 'next/navigation';
import React from 'react';

import { recordAIClick } from '@/api/ai';
import { PALETTE } from '@/constants/theme';
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
    return <div className='text-xs text-subtle py-1'>暂无相关文章</div>;
  }

  return (
    <div className='flex flex-col gap-2 my-2'>
      {articles.map((article) => (
        <button
          key={article.id}
          type='button'
          onClick={() => handleClick(article.id)}
          className='flex items-start gap-2.5 w-full p-3 bg-[#F9FAFB] rounded-lg border border-line text-left cursor-pointer hover:border-primary hover:bg-primary/[0.02] active:scale-[0.99] transition-all'
        >
          <FileTextOutlined
            style={{
              fontSize: 14,
              color: PALETTE.primary,
              marginTop: 2,
              flexShrink: 0,
            }}
          />
          <div className='min-w-0'>
            <div className='text-[13px] font-semibold text-ink truncate'>
              {article.title}
            </div>
            {article.abstract && (
              <div className='text-[12px] text-muted mt-0.5 line-clamp-2 leading-relaxed'>
                {article.abstract}
              </div>
            )}
          </div>
        </button>
      ))}
    </div>
  );
};
