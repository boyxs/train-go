'use client';

import { ExclamationCircleOutlined, PlusOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Empty,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useRouter } from 'next/navigation';
import React, { useCallback, useState } from 'react';

import * as articleApi from '@/api/article';
import { Loading } from '@/components/common/Loading';
import { useRequest } from '@/hooks/useRequest';
import type { Article } from '@/types';
import { ArticleStatus } from '@/types';

const { Text } = Typography;

const statusMap: Record<ArticleStatus, { label: string; color: string }> = {
  [ArticleStatus.Unknown]: { label: '未知', color: 'default' },
  [ArticleStatus.Unpublished]: { label: '草稿', color: 'orange' },
  [ArticleStatus.Published]: { label: '已发布', color: 'green' },
  [ArticleStatus.Private]: { label: '仅自己可见', color: 'blue' },
};

function ArticleListPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [refreshKey, setRefreshKey] = useState(0);

  const { data: listRes, loading } = useRequest(
    () => articleApi.pageArticles({ page, pageSize }),
    [page, pageSize, refreshKey],
  );

  const articles = listRes?.data?.list ?? [];
  const total = listRes?.data?.total ?? 0;

  // 刷新列表；如果当前页只剩 1 条且不是第 1 页，回退一页
  const refreshAfterRemove = useCallback(() => {
    if (articles.length <= 1 && page > 1) {
      setPage((p) => p - 1);
    }
    setRefreshKey((k) => k + 1);
  }, [articles.length, page]);

  const refresh = useCallback(() => setRefreshKey((k) => k + 1), []);

  const confirmWithdraw = (id: number) => {
    Modal.confirm({
      title: '确认撤回',
      icon: <ExclamationCircleOutlined />,
      content: '撤回后文章将从线上移除，可重新发布。',
      okText: '撤回',
      cancelText: '取消',
      okButtonProps: { danger: true },
      async onOk() {
        const res = await articleApi.withdrawArticle({ id });
        if (res.data.code === 0 || !res.data.code) {
          message.success('撤回成功');
          refresh();
        } else {
          message.error(res.data.msg || '撤回失败');
        }
      },
    });
  };

  const confirmDelete = (id: number) => {
    Modal.confirm({
      title: '确认删除',
      icon: <ExclamationCircleOutlined />,
      content: '删除后无法恢复，确定要永久删除这篇文章吗？',
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      async onOk() {
        const res = await articleApi.deleteArticle(id);
        if (res.data.code === 0 || !res.data.code) {
          message.success('删除成功');
          refreshAfterRemove();
        } else {
          message.error(res.data.msg || '删除失败');
        }
      },
    });
  };

  const columns: ColumnsType<Article> = [
    {
      title: '标题',
      dataIndex: 'Title',
      key: 'title',
      ellipsis: true,
      render: (title: string) => <Text strong>{title}</Text>,
    },
    {
      title: '状态',
      dataIndex: 'Status',
      key: 'status',
      width: 120,
      render: (status: ArticleStatus) => {
        const info = statusMap[status] || statusMap[ArticleStatus.Unknown];
        return <Tag color={info.color}>{info.label}</Tag>;
      },
    },
    {
      title: '更新时间',
      dataIndex: 'UpdatedAt',
      key: 'updatedAt',
      width: 180,
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space>
          <Button
            type='link'
            size='small'
            onClick={() => router.push(`/article/edit/${record.Id}`)}
          >
            编辑
          </Button>
          {record.Status === ArticleStatus.Published && (
            <Button
              type='link'
              size='small'
              danger
              onClick={() => confirmWithdraw(record.Id)}
            >
              撤回
            </Button>
          )}
          <Button
            type='link'
            size='small'
            danger
            onClick={() => confirmDelete(record.Id)}
          >
            删除
          </Button>
        </Space>
      ),
    },
  ];

  const extra = (
    <Button
      type='primary'
      icon={<PlusOutlined />}
      onClick={() => router.push('/article/edit')}
    >
      写文章
    </Button>
  );

  if (loading && articles.length === 0) {
    return <Loading />;
  }

  return (
    <Card title='我的文章' extra={extra}>
      {articles.length === 0 && !loading ? (
        <Empty description='暂无文章' style={{ padding: '48px 0' }}>
          <Button type='primary' onClick={() => router.push('/article/edit')}>
            写第一篇文章
          </Button>
        </Empty>
      ) : (
        <Table<Article>
          columns={columns}
          dataSource={articles}
          loading={loading}
          rowKey='Id'
          size='middle'
          pagination={{
            current: page,
            pageSize,
            total,
            showTotal: (t) => `共 ${t} 篇`,
            showSizeChanger: true,
            showQuickJumper: true,
            pageSizeOptions: ['10', '20', '50'],
            onChange: (p, ps) => {
              setPage(p);
              setPageSize(ps);
            },
          }}
        />
      )}
    </Card>
  );
}

export default ArticleListPage;
