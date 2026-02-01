import { Button, Form, Input, message } from 'antd';
import router from 'next/router';
import React from 'react';

import axios from '@/axios/axios';

const onFinish = (values: any) => {
  axios
    .post('/user/login_sms', values)
    .then((res) => {
      if (res.status !== 200) {
        message.error(res.statusText);
        return;
      }

      if (res.data.code === 0) {
        router.push('/user/profile');
        return;
      }
      message.error(res.data.msg);
    })
    .catch((err) => {
      message.error(err);
    });
};

const onFinishFailed = (_error: any) => {
  message.error('输入有误');
};

const LoginFormSMS: React.FC = () => {
  const [form] = Form.useForm();

  const sendCode = () => {
    const data = form.getFieldValue('phone');
    axios
      .post('/user/login_sms/code/send', { phone: data })
      .then((res) => {
        if (res.status !== 200) {
          message.error(res.statusText);
          return;
        }
        message.error(res?.data?.msg || '系统错误，请重试');
      })
      .catch((err) => {
        message.error(err);
      });
  };

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
      form={form}
    >
      <Form.Item
        label='手机号码'
        name='phone'
        rules={[{ required: true, message: '请输入手机号码' }]}
      >
        <Input />
      </Form.Item>

      <Form.Item
        label='验证码'
        name='code'
        rules={[{ required: true, message: '请输入验证码' }]}
      >
        <Input />
      </Form.Item>
      <Form.Item wrapperCol={{ offset: 8, span: 16 }}>
        <Button type={'default'} onClick={() => sendCode()}>
          发送验证码
        </Button>
      </Form.Item>

      <Form.Item wrapperCol={{ offset: 8, span: 16 }}>
        <Button type='primary' htmlType='submit'>
          登录/注册
        </Button>
      </Form.Item>
    </Form>
  );
};

export default LoginFormSMS;
