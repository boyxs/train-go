'use client';

import { Button } from 'antd';
import {
  ArrowUp,
  Heart,
  LoaderCircle,
  MessageCircle,
  Star,
  Users,
} from 'lucide-react';
import Link from 'next/link';
import React from 'react';

import { feedNewCount, listFeed } from '@/api/feed';
import { PALETTE } from '@/constants/theme';
import { useCursorList } from '@/hooks/useCursorList';
import { useInfiniteScroll } from '@/hooks/useInfiniteScroll';
import { useLoggedIn } from '@/hooks/useLoggedIn';
import type { FeedItem } from '@/types';
import { relativeTime } from '@/utils/format';

const CONTENT_MAX_WIDTH = 768;
const NEW_COUNT_POLL_MS = 30_000; // 提示条轮询间隔

// 引导/空态卡：白卡圆角 12 + 40px 图标 + 主/副文案 + 按钮（对齐 PRD §5）。
function GuideCard(props: {
  icon: React.ReactNode;
  title: string;
  desc: string;
  primary: React.ReactNode;
}) {
  return (
    <div
      className='bg-white rounded-xl flex flex-col items-center text-center'
      style={{ padding: '48px 24px' }}
    >
      <div style={{ color: PALETTE.faint }}>{props.icon}</div>
      <div
        style={{
          fontSize: 15,
          fontWeight: 600,
          color: PALETTE.muted,
          marginTop: 16,
        }}
      >
        {props.title}
      </div>
      <div
        style={{
          fontSize: 13,
          color: PALETTE.subtle,
          marginTop: 8,
          marginBottom: 20,
        }}
      >
        {props.desc}
      </div>
      {props.primary}
    </div>
  );
}

// 互动计数（heart/star/message-circle 14px + 数字 12px/500，同 #9CA3AF）。
function Metric({ icon, value }: { icon: React.ReactNode; value: number }) {
  return (
    <span
      className='flex items-center gap-1'
      style={{ color: PALETTE.subtle, fontSize: 12, fontWeight: 500 }}
    >
      {icon}
      {value}
    </span>
  );
}

function FeedCard({ item }: { item: FeedItem }) {
  return (
    <Link
      href={`/article/${item.articleId}`}
      target='_blank'
      className='no-underline group block'
    >
      <div className='bg-white rounded-xl px-6 py-5 hover:bg-gray-50 transition-colors'>
        <div
          className='group-hover:text-primary transition-colors'
          style={{ fontSize: 16, fontWeight: 700, color: PALETTE.ink }}
        >
          {item.title}
        </div>
        {item.abstract && (
          <div
            style={{
              fontSize: 14,
              lineHeight: 1.5,
              color: PALETTE.muted,
              marginTop: 8,
            }}
          >
            {item.abstract}
          </div>
        )}

        {/* 标签 chips（pill：圆角 12、底 #0D948815、文字主色 12px/500、padding 4×10）*/}
        {item.tags.length > 0 && (
          <div className='flex flex-wrap gap-2' style={{ marginTop: 12 }}>
            {item.tags.map((t) => (
              <span
                key={t.id}
                style={{
                  borderRadius: 12,
                  backgroundColor: `${PALETTE.primary}15`,
                  color: PALETTE.primary,
                  fontSize: 12,
                  fontWeight: 500,
                  padding: '4px 10px',
                }}
              >
                {t.name}
              </span>
            ))}
          </div>
        )}

        <div className='flex items-center gap-4' style={{ marginTop: 12 }}>
          <span
            style={{ color: PALETTE.subtle, fontSize: 12, fontWeight: 500 }}
          >
            {item.author.nickname || '匿名'}
          </span>
          <span
            style={{ color: PALETTE.subtle, fontSize: 12, fontWeight: 500 }}
          >
            {relativeTime(item.publishedAt)}
          </span>
          <Metric icon={<Heart size={14} />} value={item.likeCnt} />
          <Metric icon={<Star size={14} />} value={item.collectCnt} />
          <Metric icon={<MessageCircle size={14} />} value={item.commentCnt} />
        </div>
      </div>
    </Link>
  );
}

// FollowFeed 关注流 Tab 内容：5 态（未登录引导 / 首屏 Loading / 错误重试 / 空态引导 / 内容流）。
function FollowFeed({ onGoDiscover }: { onGoDiscover: () => void }) {
  // SSR 安全：hydrate 前不读 localStorage（否则 server render 报 localStorage is not defined）
  const { loggedIn, hydrated } = useLoggedIn();

  const fetcher = React.useCallback(
    (cursor: number, limit: number) => {
      // 未登录不发请求（避免 401），交由未登录引导态处理
      if (!loggedIn) {
        return Promise.resolve({ list: [] as FeedItem[], nextCursor: 0 });
      }
      return listFeed({ cursor, limit }).then((r) => r.data.data);
    },
    [loggedIn],
  );

  const { items, loading, loadingMore, hasMore, loadMore, error, reload } =
    useCursorList<FeedItem>(fetcher, [loggedIn]);

  // 门控含 !error：翻页失败后停止自动触发（否则 observer 重建会反复重试打爆接口），
  // 由「加载失败重试」按钮手动 loadMore（成功清 error 后自动恢复触底加载）。
  const sentinelRef = useInfiniteScroll(
    loadMore,
    loggedIn && !loading && !loadingMore && hasMore && !error,
  );

  // 新内容提示条：以当前最新条目 publishedAt 为基线，30s 轮询新增数；点击回顶刷新。
  const scrollRef = React.useRef<HTMLDivElement>(null);
  const sinceRef = React.useRef(0);
  const [newCount, setNewCount] = React.useState(0);
  const newestAt = items.length > 0 ? items[0].publishedAt : 0;

  React.useEffect(() => {
    // 列表刷新后更新基线并清零提示（loadMore 追加更旧的，newestAt 不变、不触发）
    sinceRef.current = newestAt;
    setNewCount(0);
  }, [newestAt]);

  React.useEffect(() => {
    if (!loggedIn || newestAt === 0) {
      return;
    }
    const timer = setInterval(() => {
      feedNewCount(sinceRef.current)
        .then((r) => setNewCount(r.data.data.count))
        .catch(() => {
          /* 后台轮询失败静默，不打扰浏览 */
        });
    }, NEW_COUNT_POLL_MS);
    return () => clearInterval(timer);
  }, [loggedIn, newestAt]);

  const handleShowNew = () => {
    setNewCount(0);
    scrollRef.current?.scrollTo({ top: 0 });
    void reload();
  };

  const inner = (children: React.ReactNode) => (
    <div className='flex-1 overflow-auto' ref={scrollRef}>
      <div
        className='mx-auto px-4 py-6'
        style={{ maxWidth: CONTENT_MAX_WIDTH }}
      >
        {children}
      </div>
    </div>
  );

  // 0) hydrate 前 / 首屏加载 → 加载态：pre-hydrate 不读 localStorage（规避 SSR 报错），
  //    也避免已登录用户在 hydrate 前闪现「未登录引导」。
  if (!hydrated || (loading && items.length === 0)) {
    return inner(
      <div
        className='flex items-center justify-center gap-2'
        style={{ padding: 48, color: PALETTE.subtle, fontSize: 13 }}
      >
        <LoaderCircle size={16} className='animate-spin' />
        加载中...
      </div>,
    );
  }

  // 1) 未登录引导（已 hydrate）
  if (!loggedIn) {
    return inner(
      <GuideCard
        icon={<Users size={40} />}
        title='登录后查看关注流'
        desc='关注感兴趣的作者，第一时间看到 TA 们的更新'
        primary={
          <div className='flex gap-3'>
            <Link href='/login'>
              <Button type='primary'>去登录</Button>
            </Link>
            <Button onClick={onGoDiscover}>先逛逛发现</Button>
          </div>
        }
      />,
    );
  }

  // 3) 首屏错误 + 重试
  if (error && items.length === 0) {
    return inner(
      <div
        className='bg-white rounded-xl flex flex-col items-center'
        style={{ padding: '48px 24px' }}
      >
        <div
          style={{
            fontSize: 15,
            fontWeight: 600,
            color: PALETTE.muted,
            marginBottom: 16,
          }}
        >
          {error}
        </div>
        <Button type='primary' onClick={reload}>
          重试
        </Button>
      </div>,
    );
  }

  // 4) 空态引导（已登录但未关注任何人 / 关注的人还没发文）
  if (items.length === 0) {
    return inner(
      <GuideCard
        icon={<Users size={40} />}
        title='还没有关注流内容'
        desc='去发现页关注感兴趣的作者吧'
        primary={
          <Button type='primary' onClick={onGoDiscover}>
            去发现
          </Button>
        }
      />,
    );
  }

  // 5) 内容流 + 无限滚动 + 新内容提示条
  return inner(
    <>
      {newCount > 0 && (
        <div
          className='flex justify-center'
          style={{ position: 'sticky', top: 8, zIndex: 10, marginBottom: 12 }}
        >
          <button
            type='button'
            onClick={handleShowNew}
            className='cursor-pointer border-0 flex items-center gap-1'
            style={{
              backgroundColor: PALETTE.primary,
              color: PALETTE.surface,
              fontSize: 13,
              fontWeight: 600,
              borderRadius: 9999,
              padding: '8px 16px',
            }}
          >
            <ArrowUp size={14} />有 {newCount} 篇新文章
          </button>
        </div>
      )}

      <div className='flex flex-col gap-3'>
        {items.map((item) => (
          <FeedCard key={item.articleId} item={item} />
        ))}
      </div>

      <div
        ref={sentinelRef}
        className='flex items-center justify-center gap-2'
        style={{ padding: 16, color: PALETTE.subtle, fontSize: 13 }}
      >
        {loadingMore ? (
          <>
            <LoaderCircle size={16} className='animate-spin' />
            加载中...
          </>
        ) : !hasMore ? (
          <span>没有更多了</span>
        ) : null}
      </div>

      {error && items.length > 0 && (
        <div className='text-center' style={{ paddingBottom: 16 }}>
          <Button onClick={loadMore}>加载失败，点击重试</Button>
        </div>
      )}
    </>,
  );
}

export default FollowFeed;
