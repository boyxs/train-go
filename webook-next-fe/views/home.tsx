'use client';

import { EditOutlined } from '@ant-design/icons';
import { Button, Empty, Typography } from 'antd';
import { useRouter } from 'next/navigation';
import React from 'react';

const { Title, Paragraph } = Typography;

function HomePage() {
  const router = useRouter();

  return (
    <div style={{ textAlign: 'center', paddingTop: 80 }}>
      <Title level={2}>欢迎来到小微书</Title>
      <Paragraph type='secondary'>在这里记录你的想法，分享你的故事</Paragraph>
      <Empty description='还没有文章，开始写第一篇吧' style={{ marginTop: 48 }}>
        <Button
          type='primary'
          icon={<EditOutlined />}
          size='large'
          onClick={() => router.push('/article/edit')}
        >
          写文章
        </Button>
      </Empty>
    </div>
  );
}

export default HomePage;
