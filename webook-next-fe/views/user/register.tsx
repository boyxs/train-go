'use client';

import { LockOutlined, MailOutlined } from '@ant-design/icons';
import { Button, Form, Input, message, Typography } from 'antd';
import type { AxiosError } from 'axios';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import React from 'react';

import * as userApi from '@/api/user';
import type { RegisterReq } from '@/types';

const { Title, Text } = Typography;

const RegisterForm: React.FC = () => {
  const router = useRouter();

  const onFinish = async (values: RegisterReq) => {
    try {
      const res = await userApi.register(values);
      message.success(typeof res.data === 'string' ? res.data : '注册成功');
      router.push('/login');
    } catch (err) {
      const axiosErr = err as AxiosError<string>;
      const msg = axiosErr.response?.data;
      message.error(typeof msg === 'string' && msg ? msg : '注册失败');
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
          <Text type='secondary'>创建你的账号</Text>
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

          <Form.Item
            name='confirmPassword'
            dependencies={['password']}
            rules={[
              { required: true, message: '请确认密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) {
                    return Promise.resolve();
                  }
                  return Promise.reject(new Error('两次密码输入不一致'));
                },
              }),
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder='确认密码' />
          </Form.Item>

          <Form.Item>
            <Button type='primary' htmlType='submit' block>
              注册
            </Button>
          </Form.Item>
        </Form>

        <div style={{ textAlign: 'center' }}>
          <Text type='secondary'>已有账号？</Text>{' '}
          <Link href='/login'>立即登录</Link>
        </div>
      </div>
    </div>
  );
};

export default RegisterForm;
