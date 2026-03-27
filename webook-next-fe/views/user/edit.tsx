'use client';

import { Button, Card, DatePicker, Form, Input, message, Space } from 'antd';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import { useRouter } from 'next/navigation';
import React, { useEffect } from 'react';

import * as userApi from '@/api/user';
import { Loading } from '@/components/common/Loading';
import { useRequest } from '@/hooks/useRequest';

const { TextArea } = Input;

interface FormValues {
  nickname: string;
  birthday: Dayjs;
  aboutMe: string;
}

function EditProfilePage() {
  const [form] = Form.useForm<FormValues>();
  const router = useRouter();
  const { data, loading } = useRequest(() => userApi.findProfile(), []);

  // 数据加载后填充表单（解决 initialValues 只读一次的问题）
  useEffect(() => {
    if (data) {
      form.setFieldsValue({
        nickname: data.Nickname,
        birthday: data.Birthday ? dayjs(data.Birthday) : undefined,
        aboutMe: data.AboutMe,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  const onFinish = async (values: FormValues) => {
    try {
      const res = await userApi.updateProfile({
        nickname: values.nickname,
        birthday: values.birthday ? values.birthday.valueOf() : 0,
        aboutMe: values.aboutMe,
      });
      if (res.data.code === 0) {
        message.success('修改成功');
        router.push('/user/profile');
      } else {
        message.error(res.data.msg || '修改失败');
      }
    } catch {
      message.error('系统错误');
    }
  };

  if (loading) {
    return <Loading />;
  }

  return (
    <div style={{ maxWidth: 720, margin: '0 auto' }}>
      <Card title='编辑个人信息'>
        <Form
          form={form}
          labelCol={{ span: 4 }}
          wrapperCol={{ span: 16 }}
          onFinish={onFinish}
          autoComplete='off'
        >
          <Form.Item
            label='昵称'
            name='nickname'
            rules={[{ max: 20, message: '昵称不超过 20 个字符' }]}
          >
            <Input placeholder='请输入昵称' />
          </Form.Item>

          <Form.Item label='生日' name='birthday'>
            <DatePicker
              format='YYYY-MM-DD'
              style={{ width: '100%' }}
              disabledDate={(current) => current && current > dayjs()}
            />
          </Form.Item>

          <Form.Item
            label='关于我'
            name='aboutMe'
            rules={[{ max: 200, message: '不超过 200 个字符' }]}
          >
            <TextArea
              rows={4}
              placeholder='介绍一下自己'
              showCount
              maxLength={200}
            />
          </Form.Item>

          <Form.Item wrapperCol={{ offset: 4, span: 16 }}>
            <Space>
              <Button type='primary' htmlType='submit'>
                保存
              </Button>
              <Button onClick={() => router.back()}>取消</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}

export default EditProfilePage;
