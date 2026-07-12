'use client';

import { SearchOutlined } from '@ant-design/icons';
import { Empty, Input, Pagination, Spin } from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';

import * as searchApi from '@/api/search';
import { FacetBar } from '@/components/tag/FacetBar';
import { TaggedArticleCard } from '@/components/tag/TaggedArticleCard';
import { PALETTE } from '@/constants/theme';
import { useRequest } from '@/hooks/useRequest';

import RankingBoard from './RankingBoard';

const DEFAULT_SIZE = 10;

function SearchPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const query = searchParams.get('q') ?? '';
  const page = Number(searchParams.get('page')) || 1;
  const size = Number(searchParams.get('size')) || DEFAULT_SIZE;
  const filterTags = searchParams.getAll('tag');

  const { data: res, loading } = useRequest(
    () =>
      query.trim()
        ? searchApi.searchArticles({
            query,
            page,
            size,
            filter: { tags: filterTags },
          })
        : Promise.resolve(null),
    [query, page, size, filterTags.join(',')],
  );

  const articles = res?.data?.list ?? [];
  const total = res?.data?.total ?? 0;
  const facets = res?.data?.facets ?? [];

  const buildUrl = (next: {
    page?: number;
    size?: number;
    tags?: string[];
  }) => {
    const params = new URLSearchParams();
    params.set('q', query);
    const p = next.page ?? page;
    const s = next.size ?? size;
    const t = next.tags ?? filterTags;
    if (p > 1) {
      params.set('page', String(p));
    }
    if (s !== DEFAULT_SIZE) {
      params.set('size', String(s));
    }
    t.forEach((slug) => params.append('tag', slug));
    return `/search?${params.toString()}`;
  };

  // 新查询：清空筛选与页码
  const handleSearch = (val: string) => {
    const q = val.trim();
    if (q) {
      router.push(`/search?q=${encodeURIComponent(q)}`);
    }
  };

  // 切换标签筛选 → 回第 1 页
  const handleToggleTag = (slug: string) => {
    const next = filterTags.includes(slug)
      ? filterTags.filter((s) => s !== slug)
      : [...filterTags, slug];
    router.replace(buildUrl({ tags: next, page: 1 }));
  };

  const handleClearTags = () => router.replace(buildUrl({ tags: [], page: 1 }));

  const handlePageChange = (p: number, s: number) =>
    router.replace(buildUrl({ page: p, size: s }));

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

      {!loading && !query && <RankingBoard />}

      {!loading && query && (
        <>
          <div className='mb-4 rounded-xl bg-white px-6 py-5'>
            <div className='text-base font-medium'>
              找到 <span style={{ color: PALETTE.primary }}>{total}</span> 篇「
              {query}
              」相关文章
            </div>
            {facets.length > 0 && (
              <div className='mt-3'>
                <FacetBar
                  facets={facets}
                  selected={filterTags}
                  onToggle={handleToggleTag}
                  onClear={handleClearTags}
                />
              </div>
            )}
          </div>

          {articles.length === 0 ? (
            <div className='rounded-xl bg-white p-8'>
              <Empty description={`没有找到与「${query}」相关的文章`} />
            </div>
          ) : (
            <>
              <div className='flex flex-col gap-3'>
                {articles.map((a) => (
                  <TaggedArticleCard
                    key={a.id}
                    article={a}
                    activeTags={filterTags}
                  />
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
                  pageSizeOptions={['10', '20', '50']}
                  size='small'
                  onChange={handlePageChange}
                />
              </div>
            </>
          )}
        </>
      )}
    </div>
  );
}

export default SearchPage;
