'use client';

import { RightOutlined } from '@ant-design/icons';
import { Empty, Pagination, Typography } from 'antd';
import Link from 'next/link';
import React, { useState } from 'react';

import * as articleApi from '@/api/article';
import { Loading } from '@/components/common/Loading';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { useRequest } from '@/hooks/useRequest';
import type { Article } from '@/types';

const { Text } = Typography;

// 彩色圆点循环
const dotColors = ['#0D9488', '#6366F1', '#22C55E', '#FCD34D', '#D97706'];

function ArticleFeedPage() {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const { data: listRes, loading } = useRequest(
    () => articleApi.pagePublishedArticles({ page, pageSize }),
    [page, pageSize],
  );

  const articles = listRes?.data?.list ?? [];
  const total = listRes?.data?.total ?? 0;

  if (loading && articles.length === 0) {
    return (
      <>
        <PublicHeader />
        <Loading />
      </>
    );
  }

  return (
    <div className='h-screen flex flex-col overflow-hidden bg-[#f5f5f5]'>
      <PublicHeader />

      <div className='flex-1 overflow-auto'>
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
                  >
                    <div className='bg-white rounded-xl px-6 py-5 hover:bg-gray-50 transition-colors'>
                      {/* 标题 + 箭头 */}
                      <div className='flex items-start justify-between gap-3'>
                        <div className='flex-1 min-w-0'>
                          <Text
                            strong
                            style={{ fontSize: 16 }}
                            className='group-hover:text-[#0D9488] transition-colors'
                          >
                            {article.title}
                          </Text>

                          {/* 摘要 */}
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

                          {/* 元信息 */}
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
                              {article.updatedAt}
                            </span>
                          </div>
                        </div>
                        <RightOutlined className='text-gray-300 group-hover:text-[#0D9488] mt-1 text-xs shrink-0' />
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
                  onChange={(p, ps) => {
                    setPage(p);
                    setPageSize(ps);
                  }}
                />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export default ArticleFeedPage;
