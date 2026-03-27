'use client';

import {
  ClockCircleOutlined,
  RightOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Empty, Pagination, Typography } from 'antd';
import Link from 'next/link';
import React, { useState } from 'react';

import * as articleApi from '@/api/article';
import { Loading } from '@/components/common/Loading';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { useRequest } from '@/hooks/useRequest';
import type { Article } from '@/types';

const { Text } = Typography;

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
        <div className='max-w-3xl mx-auto px-4 py-4'>
          {articles.length === 0 && !loading ? (
            <div className='bg-white rounded-lg p-8'>
              <Empty description='暂无公开文章' />
            </div>
          ) : (
            <>
              {articles.map(
                (article: Article & { abstract?: string }, idx: number) => (
                  <Link
                    key={article.id}
                    href={`/article/${article.id}`}
                    className='no-underline group block'
                  >
                    <div
                      className={`bg-white px-5 py-4 hover:bg-gray-50 transition-colors ${idx === 0 ? 'rounded-t-lg' : ''} ${idx === articles.length - 1 ? 'rounded-b-lg' : 'border-b border-gray-50'}`}
                    >
                      {/* 标题 + 箭头 */}
                      <div className='flex items-start justify-between gap-3'>
                        <div className='flex-1 min-w-0'>
                          <Text strong className='text-base'>
                            {article.title}
                          </Text>

                          {/* 摘要 */}
                          {article.abstract && (
                            <div className='mt-1.5'>
                              <Text type='secondary' className='text-sm'>
                                {article.abstract}
                              </Text>
                            </div>
                          )}

                          {/* 元信息 */}
                          <div className='flex items-center gap-3 mt-2 text-xs text-gray-400'>
                            <span className='flex items-center gap-1'>
                              <Avatar
                                size={14}
                                icon={<UserOutlined />}
                                style={{ backgroundColor: '#1677ff' }}
                              />
                              作者
                            </span>
                            <span className='flex items-center gap-1'>
                              <ClockCircleOutlined />
                              {article.updatedAt}
                            </span>
                          </div>
                        </div>
                        <RightOutlined className='text-gray-300 group-hover:text-blue-400 mt-1.5 text-xs shrink-0' />
                      </div>
                    </div>
                  </Link>
                ),
              )}

              <div className='flex justify-center mt-5'>
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
