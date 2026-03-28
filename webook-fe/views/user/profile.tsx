'use client';

import { EditOutlined, MailOutlined, PhoneOutlined } from '@ant-design/icons';
import {
  Avatar,
  Button,
  Card,
  Descriptions,
  Empty,
  Space,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import { useRouter } from 'next/navigation';
import React from 'react';

import * as userApi from '@/api/user';
import { Loading } from '@/components/common/Loading';
import { useRequest } from '@/hooks/useRequest';

const { Title, Text } = Typography;

function ProfilePage() {
  const router = useRouter();
  const { data, loading } = useRequest(() => userApi.findProfile(), []);

  if (loading) {
    return <Loading />;
  }
  if (!data) {
    return (
      <Empty description='暂无个人信息' className='mt-20'>
        <Button type='primary' onClick={() => router.push('/user/edit')}>
          完善个人信息
        </Button>
      </Empty>
    );
  }

  return (
    <div className='max-w-2xl mx-auto'>
      <Card>
        {/* 头部：头像 + 基本信息 */}
        <div className='flex flex-col items-center gap-4 mb-6 md:flex-row md:items-center md:gap-6'>
          <Avatar
            size={64}
            className='shrink-0'
            style={{ backgroundColor: '#0D9488', fontSize: 24 }}
          >
            {data.nickname?.[0]?.toUpperCase() ||
              data.email?.[0]?.toUpperCase() ||
              '?'}
          </Avatar>
          <div className='flex-1 min-w-0 text-center md:text-left'>
            <Title level={4} style={{ margin: 0 }}>
              {data.nickname || '未设置昵称'}
            </Title>
            <Space size='middle' className='mt-1' wrap>
              {data.email && (
                <Text type='secondary'>
                  <MailOutlined className='mr-1' />
                  {data.email}
                </Text>
              )}
              {data.phone && (
                <Text type='secondary'>
                  <PhoneOutlined className='mr-1' />
                  {data.phone}
                </Text>
              )}
            </Space>
          </div>
          <Button
            type='primary'
            icon={<EditOutlined />}
            onClick={() => router.push('/user/edit')}
          >
            编辑资料
          </Button>
        </div>

        {/* 详细信息 */}
        <Descriptions column={1} bordered size='small'>
          <Descriptions.Item label='昵称'>
            {data.nickname || '-'}
          </Descriptions.Item>
          <Descriptions.Item label='邮箱'>
            {data.email || '-'}
          </Descriptions.Item>
          <Descriptions.Item label='手机'>
            {data.phone || '-'}
          </Descriptions.Item>
          <Descriptions.Item label='生日'>
            {data.birthday ? dayjs(data.birthday).format('YYYY-MM-DD') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label='关于我'>
            {data.aboutMe || '-'}
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}

export default ProfilePage;
