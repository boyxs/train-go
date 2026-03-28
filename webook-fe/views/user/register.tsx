'use client';

import { LockOutlined, MailOutlined } from '@ant-design/icons';
import { App, Button, Form, Input, Typography } from 'antd';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import React from 'react';

import * as userApi from '@/api/user';
import type { RegisterReq } from '@/types';

const { Title, Text } = Typography;

const RegisterForm: React.FC = () => {
  const router = useRouter();
  const { message } = App.useApp();

  const onFinish = async (values: RegisterReq) => {
    try {
      const res = await userApi.register(values);
      if (res.data.code === 0 || !res.data.code) {
        message.success(res.data.msg || '注册成功');
        router.push('/login');
      } else {
        message.error(res.data.msg || '注册失败');
      }
    } catch {
      message.error('系统错误');
    }
  };

  return (
    <div className='min-h-screen flex justify-center items-center bg-gradient-to-br from-blue-100 to-blue-50 px-4'>
      <div className='w-full max-w-[400px] p-6 md:p-10 bg-white rounded-xl shadow-md'>
        <div className='text-center mb-6 md:mb-8'>
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

        <div className='text-center'>
          <Text type='secondary'>已有账号？</Text>{' '}
          <Link href='/login'>立即登录</Link>
        </div>

        <div className='text-center mt-3'>
          <Link href='/feed' className='text-xs text-gray-400'>
            先逛逛文章广场 →
          </Link>
        </div>
      </div>
    </div>
  );
};

export default RegisterForm;
