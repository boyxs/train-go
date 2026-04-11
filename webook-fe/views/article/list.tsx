'use client';

import { ExclamationCircleOutlined, PlusOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Empty,
  Pagination,
  Space,
  Table,
  Tag,
  Typography,
  App,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { Eye } from 'lucide-react';
import { useRouter, useSearchParams } from 'next/navigation';
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
  const searchParams = useSearchParams();
  const { message, modal } = App.useApp();

  const initPage = Number(searchParams.get('page')) || 1;
  const initSize = Number(searchParams.get('pageSize')) || 10;
  const [page, setPage] = useState(initPage);
  const [pageSize, setPageSize] = useState(initSize);
  const [refreshKey, setRefreshKey] = useState(0);

  const { data: listRes, loading } = useRequest(
    () => articleApi.pageArticles({ page, pageSize }),
    [page, pageSize, refreshKey],
  );

  const articles = listRes?.data?.list ?? [];
  const total = listRes?.data?.total ?? 0;

  const refreshAfterRemove = useCallback(() => {
    if (articles.length <= 1 && page > 1) {
      const newPage = page - 1;
      setPage(newPage);
      router.replace(`/article/list?page=${newPage}&pageSize=${pageSize}`);
    }
    setRefreshKey((k) => k + 1);
  }, [articles.length, page, pageSize, router]);

  const refresh = useCallback(() => setRefreshKey((k) => k + 1), []);

  const confirmWithdraw = (id: number) => {
    modal.confirm({
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
    modal.confirm({
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

  // ===== 桌面端 Table 列 =====
  const columns: ColumnsType<Article> = [
    {
      title: '标题',
      dataIndex: 'title',
      key: 'title',
      ellipsis: true,
      render: (title: string) => <Text strong>{title}</Text>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: ArticleStatus) => {
        const info = statusMap[status] || statusMap[ArticleStatus.Unknown];
        return <Tag color={info.color}>{info.label}</Tag>;
      },
    },
    {
      title: '阅读量',
      dataIndex: 'readCnt',
      key: 'readCnt',
      width: 120,
      render: (v?: number) =>
        v ? (
          <span className='flex items-center gap-1 text-[#1A1A1A]'>
            <Eye size={14} color='#9CA3AF' />
            {v.toLocaleString()}
          </span>
        ) : (
          <span className='text-[#D1D5DB]'>—</span>
        ),
    },
    {
      title: '更新时间',
      dataIndex: 'updatedAt',
      key: 'updatedAt',
      width: 180,
      render: (v: number) => dayjs(v).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      render: (_, record) => (
        <Space size={0}>
          <Button
            type='link'
            size='small'
            onClick={() => router.push(`/article/edit/${record.id}`)}
          >
            编辑
          </Button>
          {record.status === ArticleStatus.Published && (
            <Button
              type='link'
              size='small'
              danger
              onClick={() => confirmWithdraw(record.id)}
            >
              撤回
            </Button>
          )}
          <Button
            type='link'
            size='small'
            danger
            onClick={() => confirmDelete(record.id)}
          >
            删除
          </Button>
        </Space>
      ),
    },
  ];

  // ===== 移动端卡片 =====
  const renderMobileCard = (article: Article) => {
    const info = statusMap[article.status] || statusMap[ArticleStatus.Unknown];
    return (
      <div
        key={article.id}
        className='bg-white rounded-xl p-4 flex flex-col gap-2.5'
        style={{
          boxShadow: '0 1px 4px rgba(0,0,0,0.04)',
        }}
      >
        {/* 第一行：标题 */}
        <Text strong className='text-[15px] truncate'>
          {article.title}
        </Text>

        {/* 第二行：状态 + 日期 */}
        <div className='flex items-center gap-2'>
          <Tag
            color={info.color}
            className='m-0'
            style={{
              borderRadius: 4,
              padding: '0 8px',
              lineHeight: '20px',
              minWidth: 80,
              textAlign: 'center',
            }}
          >
            {info.label}
          </Tag>
          <span className='text-xs text-[#9CA3AF]'>
            {dayjs(article.updatedAt).format('YYYY-MM-DD')}
          </span>
        </div>

        {/* 第三行：阅读量（左）+ 操作（右） */}
        <div className='flex items-center justify-between'>
          {article.readCnt ? (
            <span className='flex items-center gap-1 text-xs text-[#9CA3AF]'>
              <Eye size={14} />
              {article.readCnt.toLocaleString()} 次阅读
            </span>
          ) : (
            <span className='text-xs text-[#D1D5DB]'>— 无阅读量</span>
          )}
          <div className='flex items-center gap-3'>
            <button
              className='text-[13px] text-[#0D9488] bg-transparent border-none cursor-pointer p-0'
              onClick={() => router.push(`/article/edit/${article.id}`)}
            >
              编辑
            </button>
            {article.status === ArticleStatus.Published ? (
              <button
                className='text-[13px] text-[#D97706] bg-transparent border-none cursor-pointer p-0'
                onClick={() => confirmWithdraw(article.id)}
              >
                撤回
              </button>
            ) : (
              <button
                className='text-[13px] text-[#0D9488] bg-transparent border-none cursor-pointer p-0'
                onClick={() => router.push(`/article/edit/${article.id}`)}
              >
                发布
              </button>
            )}
            <button
              className='text-[13px] text-[#EF4444] bg-transparent border-none cursor-pointer p-0'
              onClick={() => confirmDelete(article.id)}
            >
              删除
            </button>
          </div>
        </div>
      </div>
    );
  };

  const extra = (
    <Button
      type='primary'
      icon={<PlusOutlined />}
      onClick={() => router.push('/article/edit')}
    >
      写文章
    </Button>
  );

  const paginationProps = {
    current: page,
    pageSize,
    total,
    showTotal: (t: number) => `共 ${t} 篇`,
    showSizeChanger: true,
    showQuickJumper: true,
    pageSizeOptions: ['10', '20', '50'],
    size: 'small' as const,
    onChange: (p: number, ps: number) => {
      setPage(p);
      setPageSize(ps);
      router.replace(`/article/list?page=${p}&pageSize=${ps}`);
    },
  };

  if (loading && articles.length === 0) {
    return <Loading />;
  }

  return (
    <>
      {/* 桌面端 */}
      <div className='hidden md:block'>
        <Card title='我的文章' extra={extra}>
          {articles.length === 0 && !loading ? (
            <Empty description='暂无文章' className='py-12'>
              <Button
                type='primary'
                onClick={() => router.push('/article/edit')}
              >
                写第一篇文章
              </Button>
            </Empty>
          ) : (
            <Table<Article>
              columns={columns}
              dataSource={articles}
              loading={loading}
              rowKey='id'
              size='small'
              bordered
              pagination={paginationProps}
            />
          )}
        </Card>
      </div>

      {/* 移动端 */}
      <div className='block md:hidden'>
        <div className='flex items-center justify-between mb-3'>
          <h2 className='text-lg font-bold text-[#1A1A1A] m-0'>我的文章</h2>
          {extra}
        </div>
        {articles.length === 0 && !loading ? (
          <div className='bg-white rounded-xl p-8'>
            <Empty description='暂无文章'>
              <Button
                type='primary'
                onClick={() => router.push('/article/edit')}
              >
                写第一篇文章
              </Button>
            </Empty>
          </div>
        ) : (
          <>
            <div className='flex flex-col gap-3'>
              {articles.map(renderMobileCard)}
            </div>
            <div className='flex justify-center mt-4'>
              <Pagination {...paginationProps} simple />
            </div>
          </>
        )}
      </div>
    </>
  );
}

export default ArticleListPage;
