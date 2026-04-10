'use client';

import { Column } from '@ant-design/charts';
import {
  BarChartOutlined,
  FileTextOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  TrophyOutlined,
} from '@ant-design/icons';
import { App, Card, Spin, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CalendarDays, TrendingUp } from 'lucide-react';
import React, { useEffect, useState } from 'react';

import { fetchAIDashboard } from '@/api/ai';
import type { AIClickDashboard, TopArticle } from '@/types';

const statCards = [
  {
    key: 'totalClicks',
    label: '总点击次数',
    icon: <ThunderboltOutlined />,
    color: '#0D9488',
  },
  {
    key: 'uniqueUsers',
    label: '独立用户数',
    icon: <TeamOutlined />,
    color: '#6366F1',
  },
  {
    key: 'uniqueArticles',
    label: '独立文章数',
    icon: <FileTextOutlined />,
    color: '#D97706',
  },
  {
    key: 'avgClicksPerUser',
    label: '人均点击',
    icon: <BarChartOutlined />,
    color: '#EF4444',
  },
] as const;

const columns: ColumnsType<TopArticle> = [
  {
    title: '排名',
    dataIndex: 'rank',
    width: 64,
    align: 'center',
    render: (rank: number) =>
      rank <= 3 ? (
        <Tag
          color={rank === 1 ? '#0D9488' : rank === 2 ? '#6366F1' : '#D97706'}
          style={{ minWidth: 24, textAlign: 'center', margin: 0 }}
        >
          {rank}
        </Tag>
      ) : (
        <span className='text-[#9CA3AF]'>{rank}</span>
      ),
  },
  {
    title: '文章标题',
    dataIndex: 'title',
    ellipsis: true,
    render: (title: string, record: TopArticle) => (
      <a
        href={`/article/${record.articleId}`}
        target='_blank'
        rel='noopener noreferrer'
        className='text-[#1A1A1A] hover:text-[#0D9488] transition-colors'
      >
        {title}
      </a>
    ),
  },
  {
    title: '点击次数',
    dataIndex: 'clicks',
    width: 100,
    align: 'right',
    sorter: (a, b) => a.clicks - b.clicks,
    render: (v: number) => (
      <span className='font-semibold text-[#1A1A1A]'>{v}</span>
    ),
  },
  {
    title: '独立用户',
    dataIndex: 'uniqueUsers',
    width: 100,
    align: 'right',
    render: (v: number) => <span className='text-[#6B7280]'>{v}</span>,
  },
];

export default function AIDashboardPage() {
  const { message } = App.useApp();
  const [data, setData] = useState<AIClickDashboard | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchAIDashboard()
      .then((res) => {
        if (res.data.code === 0) {
          setData(res.data.data);
        } else {
          message.error(res.data.msg || '加载失败');
        }
      })
      .catch(() => message.error('网络错误'))
      .finally(() => setLoading(false));
  }, [message]);

  if (loading) {
    return (
      <div className='flex items-center justify-center h-96'>
        <Spin size='large' />
      </div>
    );
  }

  if (!data || (data.totalClicks === 0 && data.uniqueUsers === 0)) {
    return (
      <div className='max-w-4xl mx-auto px-4 md:px-6 py-12'>
        <div className='text-center py-20 text-[#9CA3AF]'>
          <BarChartOutlined style={{ fontSize: 48, marginBottom: 16 }} />
          <p className='text-base'>
            暂无数据，AI 对话中点击文章卡片后将自动记录
          </p>
        </div>
      </div>
    );
  }

  const formatValue = (key: string, val: number) =>
    key === 'avgClicksPerUser' ? val.toFixed(1) : val.toLocaleString();

  const chartConfig = {
    data: data.dailyTrend,
    xField: 'date',
    yField: 'clicks',
    color: '#0D9488',
    columnStyle: { radius: [4, 4, 0, 0] },
    label: {
      position: 'top' as const,
      style: { fill: '#6B7280', fontSize: 11 },
    },
    xAxis: {
      label: {
        autoRotate: true,
        style: { fontSize: 11, fill: '#9CA3AF' },
      },
    },
    yAxis: {
      label: { style: { fontSize: 11, fill: '#9CA3AF' } },
      grid: { line: { style: { stroke: '#F3F4F6' } } },
    },
    tooltip: {
      domStyles: {
        'g2-tooltip': {
          borderRadius: '8px',
          boxShadow: '0 4px 12px rgba(0,0,0,0.08)',
        },
      },
    },
    meta: { clicks: { alias: '点击次数' }, date: { alias: '日期' } },
  };

  return (
    <div className='max-w-6xl mx-auto px-4 md:px-6 py-6'>
      {/* 标题行 */}
      <div className='flex flex-col md:flex-row md:items-center justify-between mb-6 gap-3'>
        <div>
          <h1 className='text-xl font-bold text-[#1A1A1A]'>AI 引流数据看板</h1>
          <p className='text-[13px] text-[#9CA3AF] mt-1'>
            统计用户通过 AI 对话点击文章的行为数据
          </p>
        </div>
        <div className='flex items-center gap-1.5 bg-[#F3F4F6] rounded-lg px-3 py-1.5 text-[#6B7280] text-xs font-medium w-fit'>
          <CalendarDays size={14} />
          最近 30 天
        </div>
      </div>

      {/* 概览卡片 */}
      <div className='grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-6'>
        {statCards.map((card) => (
          <Card
            key={card.key}
            size='small'
            className='!rounded-xl !border-0'
            style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04)' }}
          >
            <div className='flex items-center gap-2 mb-3'>
              <div
                className='w-8 h-8 rounded-lg flex items-center justify-center'
                style={{
                  backgroundColor: `${card.color}14`,
                  color: card.color,
                }}
              >
                {card.icon}
              </div>
              <span className='text-[13px] text-[#9CA3AF] font-medium'>
                {card.label}
              </span>
            </div>
            <div className='text-[28px] font-bold text-[#1A1A1A] leading-none'>
              {formatValue(card.key, data[card.key] as number)}
            </div>
          </Card>
        ))}
      </div>

      {/* 趋势图 */}
      <Card
        title={
          <div className='flex items-center gap-2'>
            <TrendingUp size={16} className='text-[#0D9488]' />
            <span className='font-bold'>每日点击趋势</span>
          </div>
        }
        size='small'
        className='!rounded-xl !border-0 mb-6'
        style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04)' }}
      >
        <div className='h-56 md:h-72'>
          <Column {...chartConfig} />
        </div>
      </Card>

      {/* Top 10 表格 */}
      <Card
        title={
          <div className='flex items-center gap-2'>
            <TrophyOutlined className='text-[#D97706]' />
            <span className='font-bold'>热门文章 Top 10</span>
          </div>
        }
        size='small'
        className='!rounded-xl !border-0'
        style={{ boxShadow: '0 1px 3px rgba(0,0,0,0.04)' }}
      >
        <Table<TopArticle>
          columns={columns}
          dataSource={data.topArticles}
          rowKey='articleId'
          pagination={false}
          size='small'
          bordered
        />
      </Card>
    </div>
  );
}
