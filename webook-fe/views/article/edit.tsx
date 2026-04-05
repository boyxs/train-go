'use client';

import { SendOutlined } from '@ant-design/icons';
import { App, Button, Card, Form, Input, Space } from 'antd';
import { useRouter } from 'next/navigation';
import React, { useEffect } from 'react';

import * as articleApi from '@/api/article';
import { Loading } from '@/components/common/Loading';
import { useRequest } from '@/hooks/useRequest';
import type { EditArticleReq } from '@/types';

const { TextArea } = Input;

interface ArticleEditProps {
  articleId?: string;
}

function ArticleEditPage({ articleId }: ArticleEditProps) {
  const [form] = Form.useForm<EditArticleReq>();
  const { message } = App.useApp();
  const router = useRouter();
  const isEdit = !!articleId;

  // 编辑模式下加载已有文章内容
  const { data: articleRes, loading } = useRequest(
    () =>
      isEdit
        ? articleApi.findArticle(Number(articleId))
        : Promise.resolve(null),
    [articleId],
  );

  useEffect(() => {
    const article = articleRes?.data;
    if (article) {
      form.setFieldsValue({
        title: article.title,
        abstract: article.abstract,
        content: article.content,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [articleRes]);

  const submit = async (
    apiFn: (
      data: EditArticleReq,
    ) => Promise<{ data: { code?: number; msg?: string } }>,
    successMsg: string,
  ) => {
    try {
      const values = await form.validateFields();
      const data: EditArticleReq = {
        ...values,
        id: isEdit ? Number(articleId) : undefined,
      };
      const res = await apiFn(data);
      if (res.data.code === 0 || !res.data.code) {
        message.success(successMsg);
        router.back();
      } else {
        message.error(res.data.msg || '操作失败');
      }
    } catch {
      message.error('系统错误');
    }
  };

  if (isEdit && loading) {
    return <Loading />;
  }

  return (
    <div className='max-w-2xl mx-auto'>
      <Card title={isEdit ? '编辑文章' : '写文章'}>
        <Form form={form} layout='vertical'>
          <Form.Item
            label='标题'
            name='title'
            rules={[
              { required: true, message: '请输入标题' },
              { max: 100, message: '标题不超过 100 个字符' },
            ]}
          >
            <Input placeholder='请输入文章标题' showCount maxLength={100} />
          </Form.Item>

          <Form.Item
            label='摘要（选填）'
            name='abstract'
            rules={[{ max: 256, message: '摘要不超过 256 个字符' }]}
          >
            <TextArea
              placeholder='文章摘要，不填则自动截取正文'
              autoSize={{ minRows: 2, maxRows: 4 }}
              showCount
              maxLength={256}
            />
          </Form.Item>

          <Form.Item
            label='内容'
            name='content'
            rules={[{ required: true, message: '请输入内容' }]}
          >
            <TextArea
              placeholder='请输入文章内容'
              autoSize={{ minRows: 8, maxRows: 24 }}
            />
          </Form.Item>

          <Form.Item style={{ marginBottom: 0 }}>
            <Space>
              <Button
                type='primary'
                onClick={() => submit(articleApi.editArticle, '保存成功')}
              >
                保存草稿
              </Button>
              <Button
                type='primary'
                icon={<SendOutlined />}
                style={{ backgroundColor: '#52c41a', borderColor: '#52c41a' }}
                onClick={() => submit(articleApi.publishArticle, '发布成功')}
              >
                发布
              </Button>
              <Button onClick={() => router.back()}>取消</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}

export default ArticleEditPage;
