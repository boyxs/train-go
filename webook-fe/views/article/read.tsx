'use client';

import {
  ArrowLeftOutlined,
  ClockCircleOutlined,
  EyeOutlined,
  HeartFilled,
  HeartOutlined,
  StarFilled,
  StarOutlined,
} from '@ant-design/icons';
import { App, Button, Divider, Empty, Typography } from 'antd';
import dayjs from 'dayjs';
import { useRouter } from 'next/navigation';
import React, { useEffect, useRef, useState } from 'react';

import * as articleApi from '@/api/article';
import * as interactionApi from '@/api/interaction';
import { Loading } from '@/components/common/Loading';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { useRequest } from '@/hooks/useRequest';
import type { Interaction } from '@/types';

const { Title, Paragraph, Text } = Typography;

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
    () => interactionApi.findInteraction(id),
    [articleId],
  );

  const [intrOverride, setIntrOverride] = useState<Interaction | null>(null);
  const readReported = useRef(false);
  // 服务端数据为基础，用户操作后用 override 覆盖
  const intr = intrOverride ?? intrRes?.data ?? null;

  // 文章加载成功后上报阅读量（只执行一次，useRef 防 Strict Mode 双渲染）
  useEffect(() => {
    if (res?.data && !readReported.current) {
      readReported.current = true;
      interactionApi.recordView(id).catch(() => {});
    }
  }, [res?.data, id]);

  const handleLike = async () => {
    if (!intr) {
      return;
    }
    const newLiked = !intr.liked;
    try {
      const result = await interactionApi.likeArticle({
        articleId: id,
        liked: newLiked,
      });
      if (result.data.code === 0) {
        // 基于当前 intr 快照更新，而非 intrOverride（首次可能为 null）
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
      const result = await interactionApi.collectArticle({
        articleId: id,
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
      <div className='h-screen flex flex-col overflow-hidden bg-[#f5f5f5]'>
        <PublicHeader />
        <div className='flex-1 overflow-auto'>
          <div className='max-w-3xl mx-auto px-4 py-16'>
            <Empty description='文章不存在或已被撤回'>
              <Button
                onClick={() => {
                  if (window.history.length > 1) {
                    router.back();
                  } else {
                    router.push('/');
                  }
                }}
              >
                返回
              </Button>
            </Empty>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className='h-screen flex flex-col overflow-hidden bg-[#f5f5f5]'>
      <PublicHeader />

      <div className='flex-1 overflow-auto'>
        <div className='max-w-3xl mx-auto px-4 py-6'>
          <Button
            type='text'
            icon={<ArrowLeftOutlined />}
            onClick={() =>
              window.history.length > 1 ? router.back() : router.push('/feed')
            }
            className='mb-3'
          >
            返回列表
          </Button>

          <article className='bg-white rounded-lg px-6 py-6 md:px-10 md:py-8'>
            <Title level={3} style={{ marginBottom: 8 }}>
              {article.title}
            </Title>

            <div className='flex items-center justify-between flex-wrap gap-3'>
              <div className='flex items-center gap-2 text-gray-400 text-sm'>
                <ClockCircleOutlined />
                <Text type='secondary'>
                  {dayjs(article.createdAt).format('YYYY-MM-DD HH:mm')}
                </Text>
              </div>

              {/* 互动区：阅读量 + 点赞 + 收藏 */}
              {intr && (
                <div className='flex items-center gap-4'>
                  <span className='flex items-center gap-1 text-xs text-gray-400'>
                    <EyeOutlined />
                    {intr.readCount}
                  </span>

                  <button
                    onClick={handleLike}
                    className='flex items-center gap-1 text-xs border-none bg-transparent cursor-pointer p-0'
                    style={{ color: intr.liked ? '#EF4444' : '#9CA3AF' }}
                  >
                    {intr.liked ? <HeartFilled /> : <HeartOutlined />}
                    <span>{intr.likeCount}</span>
                  </button>

                  <button
                    onClick={handleCollect}
                    className='flex items-center gap-1 text-xs border-none bg-transparent cursor-pointer p-0'
                    style={{ color: intr.collected ? '#D97706' : '#9CA3AF' }}
                  >
                    {intr.collected ? <StarFilled /> : <StarOutlined />}
                    <span>{intr.collectCount}</span>
                  </button>
                </div>
              )}
            </div>

            <Divider />

            <Paragraph className='whitespace-pre-wrap leading-7 text-base'>
              {article.content}
            </Paragraph>
          </article>
        </div>
      </div>
    </div>
  );
}

export default ArticleReadPage;
