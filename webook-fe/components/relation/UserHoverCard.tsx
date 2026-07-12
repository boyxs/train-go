'use client';

import { App, Button, Popover, Spin } from 'antd';
import { Mail } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React from 'react';

import { getRelationStat } from '@/api/relation';
import { findUserInfo } from '@/api/user';
import { FollowButton } from '@/components/relation/FollowButton';
import { PALETTE } from '@/constants/theme';
import type { RelationStat, UserInfo } from '@/types';
import { formatCount } from '@/utils/format';

// 头像/昵称悬浮卡：hover 展示基本信息 + 便捷操作（关注 / 发私信）。
// 性能：数据 open 时才拉、按 userId 模块级缓存（同一作者多处 hover 只请求一次）、
// mouseEnterDelay 防止划过即触发。

interface HoverData {
  info: UserInfo;
  stat: RelationStat | null;
}

// 模块级缓存：跨组件实例共享，避免重复请求同一用户
const cache = new Map<number, HoverData>();

function CountItem({ n, label }: { n: number; label: string }) {
  return (
    <div className='flex flex-col items-center'>
      <span className='text-base font-bold text-ink'>{formatCount(n)}</span>
      <span className='text-xs text-subtle'>{label}</span>
    </div>
  );
}

interface UserHoverCardProps {
  userId: number;
  self?: boolean; // 是本人则不显示关注/私信
  children: React.ReactNode; // 触发元素（头像/昵称）
}

export function UserHoverCard({ userId, self, children }: UserHoverCardProps) {
  const router = useRouter();
  const { message } = App.useApp();
  const [data, setData] = React.useState<HoverData | null>(
    () => cache.get(userId) ?? null,
  );
  const [loading, setLoading] = React.useState(false);

  const load = async () => {
    if (data || loading) {
      return;
    }
    setLoading(true);
    try {
      const [infoRes, statRes] = await Promise.all([
        findUserInfo(userId),
        getRelationStat(userId).catch(() => null),
      ]);
      const d: HoverData = {
        info: infoRes.data.data,
        stat: statRes?.data.data ?? null,
      };
      cache.set(userId, d);
      setData(d);
    } catch {
      /* 取不到就不展示卡片主体，静默 */
    } finally {
      setLoading(false);
    }
  };

  const goProfile = () => router.push(`/user/${userId}`);

  // 关注/取关成功后：更新卡内计数 + 模块缓存（否则再次 hover 显示过期态）
  const handleChanged = (next: { isFollowing: boolean; isMutual: boolean }) => {
    setData((prev) => {
      if (!prev || !prev.stat) {
        return prev;
      }
      const delta =
        next.isFollowing === prev.stat.isFollowing
          ? 0
          : next.isFollowing
            ? 1
            : -1;
      const updated: HoverData = {
        ...prev,
        stat: {
          ...prev.stat,
          isFollowing: next.isFollowing,
          isMutual: next.isMutual,
          followerCnt: Math.max(0, prev.stat.followerCnt + delta),
        },
      };
      cache.set(userId, updated);
      return updated;
    });
  };

  const content = (
    <div style={{ width: 268 }}>
      {!data ? (
        <div className='flex justify-center py-6'>
          <Spin spinning={loading} size='small' />
          {!loading && <span className='text-xs text-subtle'>暂无信息</span>}
        </div>
      ) : (
        <div className='flex flex-col gap-3'>
          {/* 头部：头像 + 昵称 + 简介 */}
          <div className='flex cursor-pointer gap-3' onClick={goProfile}>
            <div
              className='flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg font-bold'
              style={{
                background: PALETTE.tealSurface,
                color: PALETTE.primary,
              }}
            >
              {data.info.nickname?.[0]?.toUpperCase() || '?'}
            </div>
            <div className='min-w-0 flex-1'>
              <div className='truncate text-sm font-bold text-ink'>
                {data.info.nickname || `用户 #${userId}`}
              </div>
              {data.info.aboutMe && (
                <div className='mt-0.5 line-clamp-2 text-xs text-muted'>
                  {data.info.aboutMe}
                </div>
              )}
            </div>
          </div>

          {/* 计数 */}
          <div className='flex gap-6 border-t border-hairline pt-3'>
            <CountItem n={data.stat?.followeeCnt ?? 0} label='关注' />
            <CountItem n={data.stat?.followerCnt ?? 0} label='粉丝' />
          </div>

          {/* 操作 */}
          {!self && (
            <div className='flex items-center gap-2'>
              <FollowButton
                targetId={userId}
                isFollowing={data.stat?.isFollowing ?? false}
                isMutual={data.stat?.isMutual ?? false}
                isBlocked={data.stat?.isBlocked}
                isBlockedBy={data.stat?.isBlockedBy}
                onChanged={handleChanged}
              />
              <Button
                icon={<Mail size={15} />}
                onClick={() => message.info('私信功能开发中，敬请期待')}
              >
                发私信
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );

  return (
    <Popover
      content={content}
      trigger='hover'
      mouseEnterDelay={0.4}
      mouseLeaveDelay={0.2}
      placement='bottomLeft'
      onOpenChange={(open) => {
        if (open) {
          void load();
        }
      }}
    >
      {children}
    </Popover>
  );
}
