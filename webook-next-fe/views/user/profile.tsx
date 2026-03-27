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
      <Empty description='暂无个人信息' style={{ marginTop: 80 }}>
        <Button type='primary' onClick={() => router.push('/user/edit')}>
          完善个人信息
        </Button>
      </Empty>
    );
  }

  return (
    <div style={{ maxWidth: 720, margin: '0 auto' }}>
      <Card>
        {/* 头部：头像 + 基本信息 */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 24,
            marginBottom: 24,
          }}
        >
          <Avatar
            size={72}
            style={{ backgroundColor: '#1677ff', fontSize: 28, flexShrink: 0 }}
          >
            {data.Nickname?.[0]?.toUpperCase() ||
              data.Email?.[0]?.toUpperCase() ||
              '?'}
          </Avatar>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Title level={4} style={{ margin: 0 }}>
              {data.Nickname || '未设置昵称'}
            </Title>
            <Space size='middle' style={{ marginTop: 4 }}>
              {data.Email && (
                <Text type='secondary'>
                  <MailOutlined style={{ marginRight: 4 }} />
                  {data.Email}
                </Text>
              )}
              {data.Phone && (
                <Text type='secondary'>
                  <PhoneOutlined style={{ marginRight: 4 }} />
                  {data.Phone}
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
        <Descriptions column={1} bordered size='middle'>
          <Descriptions.Item label='昵称'>
            {data.Nickname || '-'}
          </Descriptions.Item>
          <Descriptions.Item label='邮箱'>
            {data.Email || '-'}
          </Descriptions.Item>
          <Descriptions.Item label='手机'>
            {data.Phone || '-'}
          </Descriptions.Item>
          <Descriptions.Item label='生日'>
            {data.Birthday ? dayjs(data.Birthday).format('YYYY-MM-DD') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label='关于我'>
            {data.AboutMe || '-'}
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}

export default ProfilePage;
