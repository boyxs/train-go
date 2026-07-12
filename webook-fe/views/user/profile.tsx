'use client';

import { EditOutlined, MailOutlined, PhoneOutlined } from '@ant-design/icons';
import {
  Avatar,
  Button,
  Card,
  Descriptions,
  Empty,
  Space,
  Tabs,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import { useRouter, useSearchParams } from 'next/navigation';
import React from 'react';

import { getRelationStat } from '@/api/relation';
import * as userApi from '@/api/user';
import { Loading } from '@/components/common/Loading';
import { FollowList } from '@/components/relation/FollowList';
import { PALETTE } from '@/constants/theme';
import { useRequest } from '@/hooks/useRequest';
import { formatCount } from '@/utils/format';

const { Title, Text } = Typography;

type TabKey = 'following' | 'followers';

function ProfilePage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { data, loading } = useRequest(() => userApi.findProfile(), []);
  const currentUid = data?.id ?? null;

  // 计数（关注/粉丝）——profile 加载出 id 后再取；statNonce 变化时重取（关注/取关后刷新）
  const [statNonce, setStatNonce] = React.useState(0);
  const { data: statEnvelope } = useRequest(
    () => (currentUid ? getRelationStat(currentUid) : Promise.resolve(null)),
    [currentUid, statNonce],
  );
  const stat = statEnvelope?.data;
  const refreshStat = () => setStatNonce((n) => n + 1);

  const tab: TabKey =
    searchParams.get('tab') === 'followers' ? 'followers' : 'following';
  // 默认 tab（following）不写进 URL，保持干净
  const changeTab = (key: string) =>
    router.replace(
      key === 'followers' ? '/user/profile?tab=followers' : '/user/profile',
    );

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
    <div className='mx-auto flex max-w-2xl flex-col gap-3'>
      <Card>
        {/* 头部：头像 + 基本信息 */}
        <div className='mb-6 flex flex-col items-center gap-4 md:flex-row md:items-center md:gap-6'>
          <Avatar
            size={64}
            className='shrink-0'
            style={{ backgroundColor: PALETTE.primary, fontSize: 24 }}
          >
            {data.nickname?.[0]?.toUpperCase() ||
              data.email?.[0]?.toUpperCase() ||
              '?'}
          </Avatar>
          <div className='min-w-0 flex-1 text-center md:text-left'>
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

      {/* 关注与粉丝 */}
      <Card styles={{ body: { paddingTop: 8 } }}>
        <Title level={5} style={{ marginTop: 8 }}>
          关注与粉丝
        </Title>
        <Tabs
          activeKey={tab}
          onChange={changeTab}
          items={[
            {
              key: 'following',
              label: `关注 ${formatCount(stat?.followeeCnt ?? 0)}`,
              children: currentUid ? (
                <FollowList
                  userId={currentUid}
                  type='following'
                  total={stat?.followeeCnt}
                  onChanged={refreshStat}
                />
              ) : null,
            },
            {
              key: 'followers',
              label: `粉丝 ${formatCount(stat?.followerCnt ?? 0)}`,
              children: currentUid ? (
                <FollowList
                  userId={currentUid}
                  type='followers'
                  total={stat?.followerCnt}
                  onChanged={refreshStat}
                />
              ) : null,
            },
          ]}
        />
      </Card>
    </div>
  );
}

export default ProfilePage;
