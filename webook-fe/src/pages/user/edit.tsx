import { Button, DatePicker, Form, Input, message } from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import router from 'next/router';
import React, { useEffect, useMemo, useState } from 'react';

import axios from '@/axios/axios';

import { Profile } from './model';

const { TextArea } = Input;

const onFinish = (values: any) => {
  if (values.birthday) {
    values.birthday = (values.birthday as Dayjs).format('YYYY-MM-DD');
  }
  axios
    .post('/user/edit', values)
    .then((res) => {
      if (res.status !== 200) {
        message.error(res.statusText);
        return;
      }
      if (res.data?.code === 0) {
        router.push('/user/profile');
        return;
      }
      message.error(res.data?.msg || '系统错误');
    })
    .catch((err) => {
      message.error(err);
    });
};

const onFinishFailed = (_error: any) => {
  message.error('输入有误');
};

function EditForm() {
  // const [form] = Form.useForm();
  // const setData = (data: any) => {
  //   form.setFieldsValue({
  //     birthday: dayjs(data.Birthday, 'YYYY-MM-DD'),
  //     nickname: data.Nickname,
  //     aboutMe: data.AboutMe,
  //   });
  // };
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

  const initialValues = useMemo(() => {
    if (!data) {
      return {};
    }
    return {
      nickname: data.Nickname,
      birthday: data.Birthday ? dayjs(data.Birthday, 'YYYY-MM-DD') : dayjs(),
      aboutMe: data.AboutMe,
    };
  }, [data]);

  if (isLoading) {
    return <p>Loading...</p>;
  }
  if (!data) {
    return <p>No profile data</p>;
  }
  return (
    <Form
      // form={form}
      name='basic'
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
      style={{ maxWidth: 600 }}
      initialValues={initialValues}
      onFinish={onFinish}
      onFinishFailed={onFinishFailed}
      autoComplete='off'
    >
      <Form.Item label='昵称' name='nickname'>
        <Input />
      </Form.Item>

      <Form.Item label='生日' name='birthday'>
        <DatePicker format={'YYYY-MM-DD'} placeholder={''} />
      </Form.Item>

      <Form.Item label='关于我' name='aboutMe'>
        <TextArea rows={4} />
      </Form.Item>

      <Form.Item wrapperCol={{ offset: 8, span: 16 }}>
        <Button type='primary' htmlType='submit'>
          提交
        </Button>
      </Form.Item>
    </Form>
  );
}

export default EditForm;
