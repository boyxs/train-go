'use client';

import { useRouter, useSearchParams } from 'next/navigation';
import React from 'react';

import { PublicHeader } from '@/components/layout/PublicHeader';
import { PALETTE } from '@/constants/theme';
import ArticleFeedPage from '@/views/article/feed';

import FollowFeed from './FollowFeed';

const TAB_FOLLOW = 'follow';
const TAB_DISCOVER = 'discover';
type TabKey = typeof TAB_FOLLOW | typeof TAB_DISCOVER;

// 双 Tab 头（居中，间距 40；激活 15/700 主色 + 下划线 28×3；未激活 15/500 #6B7280）——对齐 PRD §5。
function TabButton({
  active,
  label,
  onClick,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type='button'
      onClick={onClick}
      className='bg-transparent border-0 cursor-pointer relative'
      style={{ padding: '0 0 8px' }}
    >
      <span
        style={{
          fontSize: 15,
          fontWeight: active ? 700 : 500,
          color: active ? PALETTE.primary : PALETTE.muted,
        }}
      >
        {label}
      </span>
      {active && (
        <span
          style={{
            position: 'absolute',
            bottom: 0,
            left: '50%',
            transform: 'translateX(-50%)',
            width: 28,
            height: 3,
            borderRadius: 2,
            backgroundColor: PALETTE.primary,
          }}
        />
      )}
    </button>
  );
}

// FeedTabsPage 广场页双 Tab 壳：关注（本模块，需登录）| 发现（原广场列表，行为原样保留）。
// Tab 状态写 URL（?tab=follow；discover 为默认省略参数）；切 tab 用 router.replace 重置列表。
function FeedTabsPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const tab: TabKey =
    searchParams.get('tab') === TAB_FOLLOW ? TAB_FOLLOW : TAB_DISCOVER;

  const switchTab = React.useCallback(
    (next: TabKey) => {
      if (next === tab) {
        return;
      }
      // follow 写 URL；discover 为默认→清空参数保持 URL 干净（同时重置发现页分页到首页）
      router.replace(next === TAB_FOLLOW ? '/feed?tab=follow' : '/feed');
    },
    [tab, router],
  );

  return (
    <div className='h-screen flex flex-col overflow-hidden bg-page'>
      <PublicHeader />
      <div
        className='flex items-center justify-center'
        style={{
          gap: 40,
          padding: '12px 0',
          backgroundColor: PALETTE.surface,
          borderBottom: `1px solid ${PALETTE.hairline}`,
        }}
      >
        <TabButton
          active={tab === TAB_FOLLOW}
          label='关注'
          onClick={() => switchTab(TAB_FOLLOW)}
        />
        <TabButton
          active={tab === TAB_DISCOVER}
          label='发现'
          onClick={() => switchTab(TAB_DISCOVER)}
        />
      </div>

      {tab === TAB_FOLLOW ? (
        <FollowFeed onGoDiscover={() => switchTab(TAB_DISCOVER)} />
      ) : (
        <ArticleFeedPage />
      )}
    </div>
  );
}

export default FeedTabsPage;
