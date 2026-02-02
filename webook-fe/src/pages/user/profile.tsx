import { ProDescriptions } from '@ant-design/pro-components';
import { Button, message, Space } from 'antd';
import router from 'next/router';
import React, { useState, useEffect } from 'react';

import axios from '@/axios/axios';

import { Profile } from './model';

function Page() {
  const [data, setData] = useState<Profile | null>(null);
  const [isLoading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    axios
      .get('/user/profile')
      .then((res) => res.data)
      .then((data) => {
        setData(data);
        setLoading(false);
      });
  }, []);

  const handleLogout = async () => {
    axios
      .post('/user/logout')
      .then((res) => res.data)
      .then((data) => {
        if (data?.code === 0) {
          document.cookie =
            'ssid=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;';
          localStorage.removeItem('access-token');
          router.push('/user/login');
          message.success(data?.msg);
        }
      });
  };

  if (isLoading) {
    return <p>Loading...</p>;
  }
  if (!data) {
    return <p>No profile data</p>;
  }

  return (
    <ProDescriptions column={1} title='个人信息'>
      <ProDescriptions.Item label='昵称' valueType='text'>
        {data.Nickname}
      </ProDescriptions.Item>
      <ProDescriptions.Item
        // span={1}
        valueType='text'
        label='邮箱'
      >
        {data.Email}
      </ProDescriptions.Item>
      <ProDescriptions.Item
        // span={1}
        valueType='text'
        label='手机'
      >
        {data.Phone}
      </ProDescriptions.Item>
      <ProDescriptions.Item label='生日' valueType='date'>
        {data.Birthday}
      </ProDescriptions.Item>
      <ProDescriptions.Item valueType='text' label='关于我'>
        {data.AboutMe}
      </ProDescriptions.Item>
      <ProDescriptions.Item>
        <Space>
          <Button href={'/user/edit'} type={'primary'}>
            修改
          </Button>
          <Button
            danger
            onClick={() => {
              handleLogout();
            }}
          >
            退出
          </Button>
        </Space>
      </ProDescriptions.Item>
    </ProDescriptions>
  );
}

export default Page;
