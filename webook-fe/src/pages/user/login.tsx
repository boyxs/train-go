import { Button, Form, Input, message } from 'antd';
import Link from 'next/link';
import router from 'next/router';
import React from 'react';

import axios from '@/axios/axios';

const onFinish = (values: any) => {
  axios
    .post('/user/login', values)
    .then((res) => {
      if (res.status !== 200) {
        message.error(res.statusText);
        return;
      }
      message.info(res.data);
      router.push('/user/profile');
    })
    .catch((err) => {
      message.error(err);
    });
};

const onFinishFailed = (_error: any) => {
  message.error('输入有误');
};

const LoginForm: React.FC = () => {
  return (
    <Form
      name='basic'
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
      style={{ maxWidth: 600 }}
      initialValues={{ remember: true }}
      onFinish={onFinish}
      onFinishFailed={onFinishFailed}
      autoComplete='off'
    >
      <Form.Item
        label='邮箱'
        name='email'
        rules={[{ required: true, message: '请输入邮箱' }]}
      >
        <Input />
      </Form.Item>

      <Form.Item
        label='密码'
        name='password'
        rules={[{ required: true, message: '请输入密码' }]}
      >
        <Input.Password />
      </Form.Item>

      <Form.Item wrapperCol={{ offset: 8, span: 16 }}>
        <Button type='primary' htmlType='submit'>
          登录
        </Button>
        <Link href={'/user/login_sms'}>&nbsp;&nbsp;手机号登录</Link>
        <Link href={'/user/login_wechat'}>&nbsp;&nbsp;微信扫码登录</Link>
        <Link href={'/user/register'}>&nbsp;&nbsp;注册</Link>
      </Form.Item>
    </Form>
  );
};

export default LoginForm;
