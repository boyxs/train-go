'use client';

import { LockOutlined, MailOutlined } from '@ant-design/icons';
import { Button, Divider, Form, Input, message, Typography } from 'antd';
import type { AxiosError } from 'axios';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import React from 'react';

import * as userApi from '@/api/user';
import type { LoginReq } from '@/types';

const { Title, Text } = Typography;

const LoginForm: React.FC = () => {
  const router = useRouter();
  const searchParams = useSearchParams();

  const onFinish = async (values: LoginReq) => {
    try {
      const res = await userApi.login(values);
      message.success(typeof res.data === 'string' ? res.data : '登录成功');
      const redirect = searchParams.get('redirect') || '/';
      router.replace(redirect);
    } catch (err) {
      const axiosErr = err as AxiosError<string>;
      const msg = axiosErr.response?.data;
      message.error(typeof msg === 'string' && msg ? msg : '用户名或密码错误');
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        background: 'linear-gradient(135deg, #e0e7ff 0%, #f0f5ff 100%)',
      }}
    >
      <div
        style={{
          width: 400,
          padding: '40px 32px',
          background: '#fff',
          borderRadius: 12,
          boxShadow: '0 2px 12px rgba(0,0,0,0.08)',
        }}
      >
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title level={3} style={{ margin: 0 }}>
            小微书
          </Title>
          <Text type='secondary'>登录你的账号</Text>
        </div>

        <Form onFinish={onFinish} autoComplete='off' size='large'>
          <Form.Item
            name='email'
            rules={[{ required: true, message: '请输入邮箱' }]}
          >
            <Input prefix={<MailOutlined />} placeholder='邮箱' />
          </Form.Item>

          <Form.Item
            name='password'
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder='密码' />
          </Form.Item>

          <Form.Item>
            <Button type='primary' htmlType='submit' block>
              登录
            </Button>
          </Form.Item>
        </Form>

        <Divider plain>
          <Text type='secondary' style={{ fontSize: 12 }}>
            其他登录方式
          </Text>
        </Divider>

        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            gap: 24,
            marginBottom: 16,
          }}
        >
          <Link href='/login/sms'>手机号登录</Link>
          <Link href='/login/wechat'>微信登录</Link>
        </div>

        <div style={{ textAlign: 'center' }}>
          <Text type='secondary'>还没有账号？</Text>{' '}
          <Link href='/register'>立即注册</Link>
        </div>
      </div>
    </div>
  );
};

export default LoginForm;
