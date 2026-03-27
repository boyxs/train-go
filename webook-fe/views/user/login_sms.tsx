'use client';

import { MobileOutlined, SafetyOutlined } from '@ant-design/icons';
import { Button, Form, Input, message, Typography } from 'antd';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import React from 'react';

import * as userApi from '@/api/user';
import type { SmsLoginReq } from '@/types';

const { Title, Text } = Typography;

const LoginSmsForm: React.FC = () => {
  const [form] = Form.useForm();
  const router = useRouter();

  const sendCode = async () => {
    const phone = form.getFieldValue('phone');
    if (!phone) {
      message.warning('请先输入手机号码');
      return;
    }
    try {
      const res = await userApi.sendSmsCode({ phone });
      if (res.data.code === 0 || !res.data.code) {
        message.success(res.data.msg || '发送成功');
      } else {
        message.error(res.data.msg);
      }
    } catch {
      message.error('发送失败');
    }
  };

  const onFinish = async (values: SmsLoginReq) => {
    try {
      const res = await userApi.loginSms(values);
      if (res.data.code === 0 || !res.data.code) {
        message.success(res.data.msg || '登录成功');
        router.replace('/');
      } else {
        message.error(res.data.msg);
      }
    } catch {
      message.error('登录失败');
    }
  };

  return (
    <div className='min-h-screen flex justify-center items-center bg-gradient-to-br from-blue-100 to-blue-50 px-4'>
      <div className='w-full max-w-[400px] p-6 md:p-10 bg-white rounded-xl shadow-md'>
        <div className='text-center mb-6 md:mb-8'>
          <Title level={3} style={{ margin: 0 }}>
            小微书
          </Title>
          <Text type='secondary'>手机号快捷登录</Text>
        </div>

        <Form form={form} onFinish={onFinish} autoComplete='off' size='large'>
          <Form.Item
            name='phone'
            rules={[{ required: true, message: '请输入手机号码' }]}
          >
            <Input prefix={<MobileOutlined />} placeholder='手机号码' />
          </Form.Item>

          <Form.Item
            name='code'
            rules={[{ required: true, message: '请输入验证码' }]}
          >
            <Input
              prefix={<SafetyOutlined />}
              placeholder='验证码'
              suffix={
                <Button type='link' size='small' onClick={sendCode}>
                  发送验证码
                </Button>
              }
            />
          </Form.Item>

          <Form.Item>
            <Button type='primary' htmlType='submit' block>
              登录 / 注册
            </Button>
          </Form.Item>
        </Form>

        <div className='text-center'>
          <Link href='/login'>邮箱密码登录</Link>
        </div>
      </div>
    </div>
  );
};

export default LoginSmsForm;
