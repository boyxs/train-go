'use client';

import { EditOutlined, ReadOutlined } from '@ant-design/icons';
import { Button, Space, Typography } from 'antd';
import { useRouter } from 'next/navigation';
import React from 'react';

const { Title, Paragraph } = Typography;

function HomePage() {
  const router = useRouter();

  return (
    <div className='text-center pt-12 md:pt-20'>
      <Title level={2}>欢迎来到小微书</Title>
      <Paragraph type='secondary'>在这里记录你的想法，分享你的故事</Paragraph>
      <Space size='middle' className='mt-8'>
        <Button
          type='primary'
          icon={<EditOutlined />}
          size='large'
          onClick={() => router.push('/article/edit')}
        >
          写文章
        </Button>
        <Button
          icon={<ReadOutlined />}
          size='large'
          onClick={() => router.push('/feed')}
        >
          文章广场
        </Button>
      </Space>
    </div>
  );
}

export default HomePage;
