'use client';

import { SendOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { App, Button, Card, Form, Input, Space } from 'antd';
import { useRouter } from 'next/navigation';
import React, { useCallback, useEffect, useState } from 'react';

import * as articleApi from '@/api/article';
import type { PolishResult } from '@/api/article';
import { PolishModal } from '@/components/article/PolishModal';
import { Loading } from '@/components/common/Loading';
import { useRequest } from '@/hooks/useRequest';
import type { EditArticleReq } from '@/types';
import { getErrorMessage } from '@/utils/apiError';

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

  const [polishing, setPolishing] = useState(false);
  const [polishResult, setPolishResult] = useState<PolishResult | null>(null);
  const [polishModalOpen, setPolishModalOpen] = useState(false);
  const [polishOriginal, setPolishOriginal] = useState({
    title: '',
    abstract: '',
    content: '',
  });

  const handlePolish = useCallback(async () => {
    const title = form.getFieldValue('title');
    const content = form.getFieldValue('content');
    if (!title?.trim() || !content?.trim()) {
      message.warning('请先输入标题和内容');
      return;
    }
    setPolishOriginal({
      title,
      abstract: form.getFieldValue('abstract') || '',
      content,
    });
    setPolishing(true);
    try {
      // 后端契约 HTTP status = 业务 code（ginx.Wrap）：2xx 必为成功；业务错误（含 429 润色超限）
      // 一律走 4xx/5xx → axios reject → 错误只在 catch 处理一次，无需再判 res.data.code。
      const res = await articleApi.polishArticle({ title, content });
      setPolishResult(res.data.data);
      setPolishModalOpen(true);
    } catch (e) {
      message.error(getErrorMessage(e, '润色失败'));
    } finally {
      setPolishing(false);
    }
  }, [form, message]);

  const handleAcceptPolish = useCallback(() => {
    if (polishResult) {
      form.setFieldsValue({
        title: polishResult.title,
        abstract: polishResult.abstract,
        content: polishResult.content,
      });
      setPolishModalOpen(false);
      message.success('已采纳润色结果');
    }
  }, [form, message, polishResult]);

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
      await apiFn(data);
      message.success(successMsg);
      router.push('/article/list');
    } catch (e) {
      message.error(getErrorMessage(e, '操作失败'));
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
            <Space wrap>
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
              <Button
                onClick={() =>
                  window.history.length > 1
                    ? router.back()
                    : router.push('/article/list')
                }
              >
                返回列表
              </Button>
              <Button
                icon={<ThunderboltOutlined />}
                loading={polishing}
                onClick={handlePolish}
                style={{ color: '#0D9488', borderColor: '#0D9488' }}
              >
                AI 润色
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <PolishModal
        open={polishModalOpen}
        original={polishOriginal}
        polished={polishResult}
        onAccept={handleAcceptPolish}
        onCancel={() => setPolishModalOpen(false)}
      />
    </div>
  );
}

export default ArticleEditPage;
