'use client';

import { App, Button, Empty } from 'antd';
import { RotateCcw } from 'lucide-react';
import React from 'react';

import { listBlocks, unblock } from '@/api/relation';
import { Loading } from '@/components/common/Loading';
import { UserCard } from '@/components/relation/UserCard';
import { useCursorList } from '@/hooks/useCursorList';
import type { BlockItem } from '@/types';
import { getErrorMessage } from '@/utils/apiError';
import { relativeTime } from '@/utils/format';

function BlocklistPage() {
  const { message } = App.useApp();
  const fetcher = React.useCallback(
    (cursor: number, limit: number) =>
      listBlocks({ cursor, limit }).then((r) => r.data.data),
    [],
  );
  const {
    items,
    setItems,
    loading,
    loadingMore,
    hasMore,
    loadMore,
    error,
    reload,
  } = useCursorList<BlockItem>(fetcher, []);
  const [pending, setPending] = React.useState<number | null>(null);

  const handleUnblock = async (it: BlockItem) => {
    setPending(it.id);
    try {
      await unblock(it.id);
      message.success('已取消拉黑');
      setItems((prev) => prev.filter((x) => x.id !== it.id));
      // 本页取消光了但服务端还有下一页 → 自动续拉，避免误显示「黑名单为空」
      // 用函数式更新删行（防并发复活）；续拉判断用当前渲染长度（多余/漏一次 refetch 均无害）
      if (items.length === 1 && hasMore) {
        void reload();
      }
    } catch (e) {
      message.error(getErrorMessage(e, '取消拉黑失败'));
    } finally {
      setPending(null);
    }
  };

  return (
    <div className='mx-auto max-w-2xl'>
      <h1 className='text-xl font-bold text-[#1A1A1A]'>黑名单</h1>
      <p className='mb-5 mt-1 text-sm text-[#6B7280]'>
        被拉黑的用户无法关注你、给你发私信或评论你的内容。
      </p>
      {loading ? (
        <Loading />
      ) : error && items.length === 0 ? (
        <div className='flex flex-col items-center gap-3 py-10'>
          <p className='text-sm text-[#6B7280]'>{error}</p>
          <Button onClick={reload}>重试</Button>
        </div>
      ) : items.length === 0 ? (
        <Empty description='黑名单为空' className='py-10' />
      ) : (
        <div className='flex flex-col gap-3'>
          {items.map((it) => (
            <UserCard
              key={it.id}
              id={it.id}
              name={it.name}
              muted
              sub={`已拉黑 · ${relativeTime(it.blockedAt)}`}
              right={
                <Button
                  icon={<RotateCcw size={14} />}
                  loading={pending === it.id}
                  onClick={() => handleUnblock(it)}
                >
                  取消拉黑
                </Button>
              }
            />
          ))}
          {error ? (
            <div className='flex flex-col items-center gap-2 py-2'>
              <span className='text-xs text-red-500'>{error}</span>
              <Button onClick={loadMore} loading={loadingMore}>
                重试
              </Button>
            </div>
          ) : hasMore ? (
            <div className='flex justify-center py-2'>
              <Button onClick={loadMore} loading={loadingMore}>
                加载更多
              </Button>
            </div>
          ) : (
            <div className='py-2 text-center text-xs text-gray-400'>
              已全部加载
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default BlocklistPage;
