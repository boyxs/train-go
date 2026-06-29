'use client';

import { App, Button, Empty } from 'antd';
import dayjs from 'dayjs';
import {
  ArrowLeft,
  Bookmark,
  BookmarkCheck,
  Clock,
  Eye,
  Heart,
} from 'lucide-react';
import { useRouter } from 'next/navigation';
import React, { useEffect, useRef, useState } from 'react';

import * as articleApi from '@/api/article';
import * as interactionApi from '@/api/interaction';
import { CommentSection } from '@/components/comment/CommentSection';
import { Loading } from '@/components/common/Loading';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { BIZ } from '@/constants/biz';
import { useRequest } from '@/hooks/useRequest';
import type { Interaction } from '@/types';
import { tokenUtil } from '@/utils/token';

interface ArticleReadProps {
  articleId: string;
}

function ArticleReadPage({ articleId }: ArticleReadProps) {
  const router = useRouter();
  const { message } = App.useApp();
  const id = Number(articleId);

  const { data: res, loading } = useRequest(
    () => articleApi.findPublishedArticle(id),
    [articleId],
  );

  const { data: intrRes } = useRequest(
    () => interactionApi.findInteraction({ biz: BIZ.ARTICLE, bizId: id }),
    [articleId],
  );

  // 登录用户才查个人状态，未登录跳过
  const { data: stateRes } = useRequest(
    () =>
      tokenUtil.hasToken()
        ? interactionApi.findUserState({ biz: BIZ.ARTICLE, bizId: id })
        : Promise.resolve(null),
    [articleId],
  );

  const [intrOverride, setIntrOverride] = useState<Interaction | null>(null);
  const readReported = useRef(false);

  // 合并聚合计数 + 个人状态
  const baseIntr = intrRes?.data ?? null;
  const userState = stateRes?.data ?? null;
  const mergedIntr: Interaction | null = baseIntr
    ? {
        ...baseIntr,
        liked: userState?.liked ?? false,
        collected: userState?.collected ?? false,
      }
    : null;
  const intr = intrOverride ?? mergedIntr;

  // 文章加载成功后上报阅读量（只执行一次，useRef 防 Strict Mode 双渲染）
  useEffect(() => {
    if (res?.data && !readReported.current) {
      readReported.current = true;
      interactionApi
        .recordView({ biz: BIZ.ARTICLE, bizId: id })
        .catch(() => {});
    }
  }, [res?.data, id]);

  const handleLike = async () => {
    if (!intr) {
      return;
    }
    const newLiked = !intr.liked;
    try {
      const result = await interactionApi.like({
        biz: BIZ.ARTICLE,
        bizId: id,
        liked: newLiked,
      });
      if (result.data.code === 0) {
        const base = intr;
        setIntrOverride({
          ...base,
          liked: newLiked,
          likeCount: Math.max(0, base.likeCount + (newLiked ? 1 : -1)),
        });
      } else {
        message.error(result.data.msg || '操作失败，请先登录');
      }
    } catch {
      message.error('请先登录后再操作');
    }
  };

  const handleCollect = async () => {
    if (!intr) {
      return;
    }
    const newCollected = !intr.collected;
    try {
      const result = await interactionApi.collect({
        biz: BIZ.ARTICLE,
        bizId: id,
        collected: newCollected,
      });
      if (result.data.code === 0) {
        const base = intr;
        setIntrOverride({
          ...base,
          collected: newCollected,
          collectCount: Math.max(
            0,
            base.collectCount + (newCollected ? 1 : -1),
          ),
        });
      } else {
        message.error(result.data.msg || '操作失败，请先登录');
      }
    } catch {
      message.error('请先登录后再操作');
    }
  };

  const article = res?.data;

  if (loading) {
    return (
      <>
        <PublicHeader />
        <Loading />
      </>
    );
  }

  if (!article) {
    return (
      <div className='h-screen flex flex-col overflow-hidden bg-[#F5F5F5]'>
        <PublicHeader />
        <div className='flex-1 overflow-auto'>
          <div className='max-w-3xl mx-auto px-4 py-16'>
            <Empty description='文章不存在或已被撤回'>
              <Button onClick={() => router.push('/feed')}>返回广场</Button>
            </Empty>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className='h-screen flex flex-col overflow-hidden bg-[#F5F5F5]'>
      <PublicHeader />

      <div className='flex-1 overflow-auto'>
        <div className='mx-auto px-4 py-6' style={{ maxWidth: 768 }}>
          {/* 返回广场 */}
          <button
            onClick={() =>
              window.history.length > 1 ? router.back() : router.push('/feed')
            }
            className='flex items-center gap-1 mb-4 text-[#6B7280] text-sm font-medium bg-transparent border-none cursor-pointer p-0 hover:text-[#0D9488]'
          >
            <ArrowLeft size={14} />
            返回广场
          </button>

          {/* 文章正文卡片 */}
          <div
            className='bg-white rounded-xl flex flex-col gap-4'
            style={{ padding: '32px 40px' }}
          >
            <h1 className='text-2xl font-bold text-[#1A1A1A] m-0'>
              {article.title}
            </h1>

            {/* 时间元信息 */}
            <div className='flex items-center gap-1.5'>
              <Clock size={14} color='#9CA3AF' />
              <span className='text-[13px] font-medium text-[#9CA3AF]'>
                {dayjs(article.createdAt).format('YYYY-MM-DD HH:mm')}
              </span>
            </div>

            {/* 分割线 */}
            <div className='h-px bg-[#F3F4F6]' />

            {/* 正文 */}
            <div
              className='whitespace-pre-wrap'
              style={{
                fontSize: 15,
                lineHeight: 1.8,
                color: '#444444',
              }}
            >
              {article.content}
            </div>
          </div>

          {/* 互动栏 — 独立卡片 */}
          {intr && (
            <div
              className='bg-white rounded-xl flex items-center gap-3 mt-4'
              style={{ padding: '16px 24px' }}
            >
              {/* 点赞按钮 */}
              <button
                onClick={handleLike}
                className='flex items-center gap-1.5 rounded-lg border cursor-pointer transition-colors'
                style={{
                  padding: '8px 16px',
                  backgroundColor: intr.liked ? '#FFF5F5' : 'transparent',
                  borderColor: intr.liked ? '#FFE4E4' : '#E5E7EB',
                  color: intr.liked ? '#EF4444' : '#6B7280',
                }}
              >
                <Heart
                  size={16}
                  fill={intr.liked ? '#EF4444' : 'none'}
                  color={intr.liked ? '#EF4444' : '#9CA3AF'}
                />
                <span className='text-[13px] font-semibold'>
                  点赞 {intr.likeCount}
                </span>
              </button>

              {/* 收藏按钮 */}
              <button
                onClick={handleCollect}
                className='flex items-center gap-1.5 rounded-lg border cursor-pointer transition-colors'
                style={{
                  padding: '8px 16px',
                  borderColor: intr.collected ? '#0D9488' : '#E5E7EB',
                  backgroundColor: intr.collected ? '#F0FDFA' : 'transparent',
                  color: intr.collected ? '#0D9488' : '#6B7280',
                }}
              >
                {intr.collected ? (
                  <BookmarkCheck size={16} color='#0D9488' />
                ) : (
                  <Bookmark size={16} color='#9CA3AF' />
                )}
                <span className='text-[13px] font-semibold'>
                  收藏 {intr.collectCount}
                </span>
              </button>

              {/* 阅读量 — 右对齐 */}
              <div className='flex items-center gap-1.5 ml-auto'>
                <Eye size={14} color='#D1D5DB' />
                <span className='text-xs font-medium text-[#D1D5DB]'>
                  {intr.readCount.toLocaleString()} 次阅读
                </span>
              </div>
            </div>
          )}

          {/* 评论区 */}
          <CommentSection articleId={id} />
        </div>
      </div>
    </div>
  );
}

export default ArticleReadPage;
