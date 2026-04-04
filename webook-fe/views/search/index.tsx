'use client';

import { RightOutlined, SearchOutlined } from '@ant-design/icons';
import { Empty, Input, Pagination, Spin, Typography } from 'antd';
import dayjs from 'dayjs';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import React from 'react';

import * as searchApi from '@/api/search';
import { useRequest } from '@/hooks/useRequest';
import type { Article } from '@/types';

const { Text } = Typography;

const dotColors = ['#0D9488', '#6366F1', '#22C55E', '#FCD34D', '#D97706'];

function SearchPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const query = searchParams.get('q') ?? '';
  const page = Number(searchParams.get('page')) || 1;
  const size = Number(searchParams.get('size')) || 10;

  const { data: res, loading } = useRequest(
    () =>
      query.trim()
        ? searchApi.searchArticles({ query, page, size })
        : Promise.resolve(null),
    [query, page, size],
  );

  const articles = res?.data?.list ?? [];
  const total = res?.data?.total ?? 0;

  const handleSearch = (val: string) => {
    const q = val.trim();
    if (!q) {
      return;
    }
    router.push(`/search?q=${encodeURIComponent(q)}`);
  };

  const handlePageChange = (p: number, s: number) => {
    const params = new URLSearchParams();
    params.set('q', query);
    params.set('page', String(p));
    if (s !== 10) {
      params.set('size', String(s));
    }
    router.push(`/search?${params.toString()}`);
  };

  return (
    <div className='mx-auto px-4 py-6' style={{ maxWidth: 768 }}>
      <Input.Search
        key={query}
        defaultValue={query}
        placeholder='搜索文章...'
        size='large'
        enterButton={<SearchOutlined />}
        onSearch={handleSearch}
        style={{ marginBottom: 24 }}
      />

      {loading && (
        <div className='flex justify-center py-16'>
          <Spin size='large' />
        </div>
      )}

      {!loading && query && articles.length === 0 && (
        <div className='bg-white rounded-xl p-8'>
          <Empty description={`没有找到与「${query}」相关的文章`} />
        </div>
      )}

      {!loading && !query && (
        <div className='bg-white rounded-xl p-8'>
          <Empty description='输入关键词开始搜索' />
        </div>
      )}

      {!loading && articles.length > 0 && (
        <>
          <div className='mb-3 text-sm text-gray-400'>
            共找到 <span className='text-[#0D9488] font-medium'>{total}</span>{' '}
            篇文章
          </div>
          <div className='flex flex-col gap-3'>
            {articles.map((article: Article, idx: number) => (
              <Link
                key={article.id}
                href={`/article/${article.id}`}
                className='no-underline group block'
              >
                <div className='bg-white rounded-xl px-6 py-5 hover:bg-gray-50 transition-colors'>
                  <div className='flex items-start justify-between gap-3'>
                    <div className='flex-1 min-w-0'>
                      <Text
                        strong
                        style={{ fontSize: 16 }}
                        className='group-hover:text-[#0D9488] transition-colors'
                      >
                        {article.title}
                      </Text>

                      {article.abstract && (
                        <div className='mt-2'>
                          <Text
                            type='secondary'
                            style={{ fontSize: 14, lineHeight: '1.5' }}
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
                          {article.author?.name || '作者'}
                        </span>
                        <span className='text-xs text-gray-400'>
                          {dayjs(article.createdAt).format('YYYY-MM-DD HH:mm')}
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
              pageSize={size}
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
  );
}

export default SearchPage;
