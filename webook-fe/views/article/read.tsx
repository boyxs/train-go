'use client';

import { ArrowLeftOutlined, ClockCircleOutlined } from '@ant-design/icons';
import { Button, Divider, Empty, Typography } from 'antd';
import dayjs from 'dayjs';
import { useRouter } from 'next/navigation';
import React from 'react';

import * as articleApi from '@/api/article';
import { Loading } from '@/components/common/Loading';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { useRequest } from '@/hooks/useRequest';

const { Title, Paragraph, Text } = Typography;

interface ArticleReadProps {
  articleId: string;
}

function ArticleReadPage({ articleId }: ArticleReadProps) {
  const router = useRouter();
  const { data: res, loading } = useRequest(
    () => articleApi.findPublishedArticle(Number(articleId)),
    [articleId],
  );

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
              <Button onClick={() => router.back()}>返回</Button>
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
            onClick={() => router.back()}
            className='mb-3'
          >
            返回列表
          </Button>

          <article className='bg-white rounded-lg px-6 py-6 md:px-10 md:py-8'>
            <Title level={3} style={{ marginBottom: 8 }}>
              {article.title}
            </Title>

            <div className='flex items-center gap-2 text-gray-400 text-sm'>
              <ClockCircleOutlined />
              <Text type='secondary'>
                {dayjs(article.updatedAt).format('YYYY-MM-DD HH:mm')}
              </Text>
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
