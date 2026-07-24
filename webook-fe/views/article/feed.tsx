'use client';

import { RightOutlined } from '@ant-design/icons';
import { Empty, Pagination, Typography } from 'antd';
import dayjs from 'dayjs';
import { Eye } from 'lucide-react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import React from 'react';

import * as articleApi from '@/api/article';
import { Loading } from '@/components/common/Loading';
import { STORAGE_KEYS } from '@/constants/storage';
import { PALETTE } from '@/constants/theme';
import { useRequest } from '@/hooks/useRequest';
import { useScrollRestore } from '@/hooks/useScrollRestore';
import type { Article } from '@/types';

const { Text } = Typography;

const dotColors = [
  PALETTE.primary,
  PALETTE.info,
  PALETTE.success,
  '#FCD34D',
  PALETTE.warning,
];

function formatReadCount(n: number): string {
  if (n >= 10000) {
    return `${(n / 10000).toFixed(1)}w`;
  }
  if (n >= 1000) {
    return `${(n / 1000).toFixed(1)}k`;
  }
  return String(n);
}

function ArticleFeedPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const page = Number(searchParams.get('page')) || 1;
  const pageSize = Number(searchParams.get('pageSize')) || 10;

  const { data: listRes, loading } = useRequest(
    () => articleApi.pagePublishedArticles({ page, pageSize }),
    [page, pageSize],
  );

  const articles = listRes?.data?.list ?? [];
  const total = listRes?.data?.total ?? 0;

  const [scrollRef, setScroll, resetScroll] = useScrollRestore(
    STORAGE_KEYS.SCROLL_FEED,
    !loading && articles.length > 0,
  );

  const handlePageChange = (p: number, ps: number) => {
    resetScroll();
    const params = new URLSearchParams();
    params.set('page', String(p));
    if (ps !== 10) {
      params.set('pageSize', String(ps));
    }
    router.push(`/feed?${params.toString()}`);
  };

  if (loading && articles.length === 0) {
    return <Loading />;
  }

  return (
    <div className='flex-1 overflow-auto' ref={scrollRef}>
      <div className='mx-auto px-4 py-6' style={{ maxWidth: 768 }}>
        {articles.length === 0 && !loading ? (
          <div className='bg-white rounded-xl p-8'>
            <Empty description='暂无公开文章' />
          </div>
        ) : (
          <>
            <div className='flex flex-col gap-3'>
              {articles.map((article: Article, idx: number) => (
                <Link
                  key={article.id}
                  href={`/article/${article.id}`}
                  className='no-underline group block'
                  onClick={setScroll}
                >
                  <div className='bg-white rounded-xl px-6 py-5 hover:bg-gray-50 transition-colors'>
                    <div className='flex items-start justify-between gap-3'>
                      <div className='flex-1 min-w-0'>
                        <Text
                          strong
                          style={{ fontSize: 16 }}
                          className='group-hover:text-primary transition-colors'
                        >
                          {article.title}
                        </Text>

                        {article.abstract && (
                          <div className='mt-2'>
                            <Text
                              type='secondary'
                              style={{
                                fontSize: 14,
                                lineHeight: '1.5',
                              }}
                            >
                              {article.abstract}
                            </Text>
                          </div>
                        )}

                        <div className='flex items-center gap-4 mt-3'>
                          <span className='flex items-center gap-1.5 text-xs text-gray-400'>
                            <span
                              className='inline-block w-4 h-4 rounded-full shrink-0'
                              style={{
                                backgroundColor:
                                  dotColors[idx % dotColors.length],
                              }}
                            />
                            作者
                          </span>
                          <span className='flex items-center gap-1.5 text-xs text-gray-400'>
                            <svg
                              width='12'
                              height='12'
                              viewBox='0 0 24 24'
                              fill='none'
                              stroke='currentColor'
                              strokeWidth='2'
                            >
                              <circle cx='12' cy='12' r='10' />
                              <polyline points='12 6 12 12 16 14' />
                            </svg>
                            {dayjs(article.createdAt).format(
                              'YYYY-MM-DD HH:mm',
                            )}
                          </span>
                          {article.readCnt ? (
                            <span className='flex items-center gap-1.5 text-xs text-gray-400'>
                              <Eye size={14} />
                              {formatReadCount(article.readCnt)}
                            </span>
                          ) : null}
                        </div>
                      </div>
                      <RightOutlined className='text-gray-300 group-hover:text-primary mt-1 text-xs shrink-0' />
                    </div>
                  </div>
                </Link>
              ))}
            </div>

            <div className='flex justify-center mt-6'>
              <Pagination
                current={page}
                pageSize={pageSize}
                total={total}
                showTotal={(t) => `共 ${t} 篇`}
                showSizeChanger
                showQuickJumper
                pageSizeOptions={['10', '20', '50']}
                size='small'
                onChange={handlePageChange}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

export default ArticleFeedPage;
