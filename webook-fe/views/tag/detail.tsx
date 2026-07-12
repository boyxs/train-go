'use client';

import { Empty, Pagination, Radio, Spin, Typography } from 'antd';
import type { RadioChangeEvent } from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import { useState } from 'react';

import { findTag, pageTagArticles } from '@/api/tag';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { TagFollowButton } from '@/components/tag/TagFollowButton';
import { TaggedArticleCard } from '@/components/tag/TaggedArticleCard';
import { PALETTE } from '@/constants/theme';
import { useRequest } from '@/hooks/useRequest';
import type { TagArticleSort } from '@/types';
import { formatCount } from '@/utils/format';

const { Title, Text } = Typography;

const DEFAULT_SIZE = 20;
const DEFAULT_SORT: TagArticleSort = 'new';

interface TagDetailProps {
  slug: string;
}

function TagDetailPage({ slug }: TagDetailProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const page = Number(searchParams.get('page')) || 1;
  const size = Number(searchParams.get('size')) || DEFAULT_SIZE;
  const sort = (searchParams.get('sort') as TagArticleSort) || DEFAULT_SORT;

  const { data: tagRes } = useRequest(() => findTag(slug), [slug]);
  const { data: listRes, loading } = useRequest(
    () => pageTagArticles(slug, { page, size, sort }),
    [slug, page, size, sort],
  );

  const tag = tagRes?.data;
  const articles = listRes?.data?.list ?? [];
  const total = listRes?.data?.total ?? 0;

  // 关注数：服务端值为基准，关注/取关成功后用本地乐观值覆盖（按 slug 隔离，切标签自动复位）
  const [followOverride, setFollowOverride] = useState<{
    slug: string;
    count: number;
  } | null>(null);
  const followCount =
    followOverride && followOverride.slug === tag?.slug
      ? followOverride.count
      : (tag?.followCount ?? 0);

  const setParams = (next: {
    page?: number;
    size?: number;
    sort?: TagArticleSort;
  }) => {
    const params = new URLSearchParams();
    const p = next.page ?? page;
    const s = next.size ?? size;
    const so = next.sort ?? sort;
    if (p > 1) {
      params.set('page', String(p));
    }
    if (s !== DEFAULT_SIZE) {
      params.set('size', String(s));
    }
    if (so !== DEFAULT_SORT) {
      params.set('sort', so);
    }
    const qs = params.toString();
    router.replace(`/tag/${encodeURIComponent(slug)}${qs ? `?${qs}` : ''}`);
  };

  return (
    <div className='flex h-screen flex-col overflow-hidden bg-page'>
      <PublicHeader />
      <div className='flex-1 overflow-auto'>
        <div className='mx-auto px-4 py-6' style={{ maxWidth: 768 }}>
          <div className='mb-4 rounded-xl bg-white px-6 py-5'>
            <div className='flex items-start justify-between gap-4'>
              <div className='min-w-0'>
                <Title level={3} style={{ margin: 0 }}>
                  <span style={{ color: PALETTE.primary }}># </span>
                  {tag?.name ?? slug}
                </Title>
                <div className='mt-1 text-sm text-gray-400'>
                  {tag?.refCount ?? 0} 篇内容
                  <span className='mx-1.5'>·</span>
                  {formatCount(followCount)} 人关注
                  {tag && tag.weeklyNewCount > 0 && (
                    <>
                      <span className='mx-1.5'>·</span>
                      本周新增 {formatCount(tag.weeklyNewCount)} 篇
                    </>
                  )}
                </div>
              </div>
              {tag && (
                <TagFollowButton
                  key={slug}
                  slug={slug}
                  isFollowing={tag.isFollowing}
                  onChanged={(next) =>
                    setFollowOverride({ slug, count: next.followCount })
                  }
                />
              )}
            </div>
            {tag?.description && (
              <div className='mt-2'>
                <Text type='secondary'>{tag.description}</Text>
              </div>
            )}
          </div>

          <div className='mb-4'>
            <Radio.Group
              value={sort}
              buttonStyle='solid'
              onChange={(e: RadioChangeEvent) =>
                setParams({ sort: e.target.value as TagArticleSort, page: 1 })
              }
            >
              <Radio.Button value='new'>最新</Radio.Button>
              <Radio.Button value='hot'>最热</Radio.Button>
            </Radio.Group>
          </div>

          {loading && (
            <div className='flex justify-center py-16'>
              <Spin size='large' />
            </div>
          )}

          {!loading && articles.length === 0 && (
            <div className='rounded-xl bg-white p-8'>
              <Empty description='该标签下暂无文章' />
            </div>
          )}

          {!loading && articles.length > 0 && (
            <>
              <div className='flex flex-col gap-3'>
                {articles.map((a) => (
                  <TaggedArticleCard key={a.id} article={a} />
                ))}
              </div>
              <div className='mt-6 flex justify-center'>
                <Pagination
                  current={page}
                  pageSize={size}
                  total={total}
                  showTotal={(t) => `共 ${t} 条`}
                  showSizeChanger
                  showQuickJumper
                  pageSizeOptions={['20', '50']}
                  size='small'
                  onChange={(p, s) => setParams({ page: p, size: s })}
                />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export default TagDetailPage;
