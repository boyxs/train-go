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

        <div style={{ textAlign: 'center' }}>
          <Link href='/login'>邮箱密码登录</Link>
        </div>
      </div>
    </div>
  );
};

export default LoginSmsForm;
