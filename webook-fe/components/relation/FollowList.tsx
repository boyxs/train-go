'use client';

import { Button, Empty } from 'antd';
import { useRouter } from 'next/navigation';
import React from 'react';

import { listFollowees, listFollowers } from '@/api/relation';
import { Loading } from '@/components/common/Loading';
import { FollowButton } from '@/components/relation/FollowButton';
import { UserCard } from '@/components/relation/UserCard';
import { useCursorList } from '@/hooks/useCursorList';
import type { FolloweeItem, FollowerItem } from '@/types';
import { formatCount, relativeTime } from '@/utils/format';

interface FollowListProps {
  userId: number; // 列表主人（= 当前登录用户）
  type: 'following' | 'followers';
  total?: number; // 该 Tab 总数（用于「已加载 X / 总数」脚注）
  onChanged?: () => void; // 关注/取关成功后通知父级（刷新计数）
}

export function FollowList({
  userId,
  type,
  total,
  onChanged,
}: FollowListProps) {
  const router = useRouter();

  const fetcher = React.useCallback(
    (cursor: number, limit: number) =>
      (type === 'following'
        ? listFollowees({ userId, cursor, limit })
        : listFollowers({ userId, cursor, limit })
      ).then((r) => r.data.data),
    [userId, type],
  );

  const { items, loading, loadingMore, hasMore, loadMore, error, reload } =
    useCursorList<FolloweeItem | FollowerItem>(fetcher, [userId, type]);

  if (loading) {
    return <Loading />;
  }
  if (error && items.length === 0) {
    return (
      <div className='flex flex-col items-center gap-3 py-10'>
        <p className='text-sm text-muted'>{error}</p>
        <Button onClick={reload}>重试</Button>
      </div>
    );
  }
  if (items.length === 0) {
    return (
      <Empty
        description={type === 'following' ? '还没有关注任何人' : '还没有粉丝'}
        className='py-10'
      />
    );
  }

  return (
    <div className='flex flex-col gap-3'>
      {items.map((it) => {
        const isFollower = type === 'followers';
        // following：都是我关注的（isFollowing=true），互关取 isMutual
        // followers：粉丝，isFollowedBack= 我是否已回关（回关后即互关）
        const mutual = isFollower
          ? (it as FollowerItem).isFollowedBack
          : (it as FolloweeItem).isMutual;
        const isFollowing = isFollower
          ? (it as FollowerItem).isFollowedBack
          : true;
        const countPart = `${formatCount(it.followeeCnt)} 关注 · ${formatCount(it.followerCnt)} 粉丝`;
        const sub = isFollower
          ? `关注了你 · ${relativeTime((it as FollowerItem).createdAt)} · ${countPart}`
          : [it.bio, countPart].filter(Boolean).join(' · ');
        return (
          <UserCard
            key={it.id}
            id={it.id}
            name={it.name}
            sub={sub}
            mutual={mutual}
            onClick={() => router.push(`/user/${it.id}`)}
            right={
              <FollowButton
                targetId={it.id}
                isFollowing={isFollowing}
                isMutual={mutual}
                mutualOnFollow={isFollower ? true : mutual}
                followLabel={isFollower ? '回关' : undefined}
                onChanged={onChanged}
              />
            }
          />
        );
      })}
      <div className='flex flex-col items-center gap-2 py-2'>
        {error && <span className='text-xs text-red-500'>{error}</span>}
        {hasMore && (
          <Button onClick={loadMore} loading={loadingMore}>
            {error ? '重试' : '加载更多'}
          </Button>
        )}
        <span className='text-xs text-gray-400'>
          {total !== undefined
            ? `已加载 ${items.length} / ${total}`
            : hasMore
              ? ''
              : '已全部加载'}
        </span>
      </div>
    </div>
  );
}
