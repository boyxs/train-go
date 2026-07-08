'use client';

import {
  AppstoreOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  ClockCircleOutlined,
  CommentOutlined,
  EyeOutlined,
  FileTextOutlined,
  FireOutlined,
  HistoryOutlined,
  LikeOutlined,
  RiseOutlined,
  StarOutlined,
  ThunderboltOutlined,
  TrophyFilled,
  TrophyOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { App, Drawer, Empty, Grid, List, Pagination, Skeleton } from 'antd';
import dayjs from 'dayjs';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import React from 'react';

import * as rankingApi from '@/api/ranking';
import { useRequest } from '@/hooks/useRequest';
import type { ArticleRanking, Dimension } from '@/types/ranking';
import { getErrorMessage } from '@/utils/apiError';

const DIMENSIONS: { key: Dimension; label: string; icon: React.ReactNode }[] = [
  { key: 'hot', label: '热度', icon: <ThunderboltOutlined /> },
  { key: 'new', label: '最新', icon: <ClockCircleOutlined /> },
  { key: 'best', label: '最佳', icon: <TrophyOutlined /> },
  { key: 'category', label: '分区', icon: <AppstoreOutlined /> },
];

// P0 只做文章榜，其他对象 disabled 占位（pen 原型已规划）
const OBJECTS: {
  key: string;
  label: string;
  icon: React.ReactNode;
  enabled: boolean;
}[] = [
  { key: 'article', label: '文章', icon: <FileTextOutlined />, enabled: true },
  { key: 'user', label: '用户', icon: <UserOutlined />, enabled: false },
  { key: 'chat', label: 'AI 对话', icon: <CommentOutlined />, enabled: false },
  { key: 'keyword', label: '热词', icon: <FireOutlined />, enabled: false },
];

// 奖牌颜色（1/2/3 名）
const MEDAL_COLORS = ['#F59E0B', '#9CA3AF', '#B45309'];

// 距次日 00:00 的倒计时（HH:MM:SS）
function freezeCountdown(now: Date): string {
  const tomorrow = new Date(now);
  tomorrow.setHours(24, 0, 0, 0);
  const diff = tomorrow.getTime() - now.getTime();
  const h = String(Math.floor(diff / 3600000)).padStart(2, '0');
  const m = String(Math.floor((diff % 3600000) / 60000)).padStart(2, '0');
  const s = String(Math.floor((diff % 60000) / 1000)).padStart(2, '0');
  return `${h}:${m}:${s}`;
}

const CATEGORIES: { key: string; label: string; color: string; bg: string }[] =
  [
    { key: 'tech', label: '技术', color: '#6366F1', bg: '#E0E7FF' },
    { key: 'career', label: '职场', color: '#D97706', bg: '#FEF3C7' },
    { key: 'life', label: '生活', color: '#22C55E', bg: '#F0FDF4' },
    { key: 'ai', label: 'AI', color: '#0D9488', bg: '#F0FDFA' },
    { key: 'other', label: '其他', color: '#6B7280', bg: '#F3F4F6' },
  ];

// 当前 Shanghai 日期字符串 YYYY-MM-DD（业务时区），与后端 carbon.Now().ToDateString() 对齐
// 'sv-SE' locale 格式就是 YYYY-MM-DD，配合 timeZone 参数拿 Shanghai 日历日
function todayDate(): string {
  return new Date().toLocaleDateString('sv-SE', {
    timeZone: 'Asia/Shanghai',
  });
}

function formatCount(n: number) {
  if (n >= 10000) {
    return `${(n / 1000).toFixed(1)}k`;
  }
  return String(n);
}

// 按维度格式化分数：hot/category 显示整数 + k 缩写；best 显示百分比；new 显示发布时间
function formatScore(score: number, dim: Dimension): string {
  if (dim === 'best') {
    return `${(score * 100).toFixed(1)}%`;
  }
  if (dim === 'new') {
    // score 是发布毫秒时间戳
    const d = new Date(score);
    const mm = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    const hh = String(d.getHours()).padStart(2, '0');
    const mi = String(d.getMinutes()).padStart(2, '0');
    return `${mm}-${dd} ${hh}:${mi}`;
  }
  // hot / category：大数缩写
  return formatCount(Math.round(score));
}

// medal 返回 Top3 的奖杯图标（金/银/铜），非 Top3 返 null
function medal(rank: number): React.ReactNode {
  if (rank >= 1 && rank <= 3) {
    return <TrophyFilled style={{ color: MEDAL_COLORS[rank - 1] }} />;
  }
  return null;
}

function TrendBadge({ item }: { item: ArticleRanking }) {
  if (item.trend === 'new') {
    return (
      <span
        style={{
          fontSize: 9,
          fontWeight: 700,
          color: '#EF4444',
          background: '#FEF2F2',
          borderRadius: 10,
          padding: '2px 6px',
        }}
      >
        NEW
      </span>
    );
  }
  if (item.trend === 'up') {
    return (
      <span
        style={{
          fontSize: 10,
          fontWeight: 700,
          color: '#EF4444',
          background: '#FEF2F2',
          borderRadius: 10,
          padding: '2px 6px',
          display: 'inline-flex',
          alignItems: 'center',
          gap: 2,
        }}
      >
        <ArrowUpOutlined style={{ fontSize: 10 }} /> {item.trendDelta}
      </span>
    );
  }
  if (item.trend === 'down') {
    return (
      <span
        style={{
          fontSize: 10,
          fontWeight: 700,
          color: '#22C55E',
          background: '#ECFDF5',
          borderRadius: 10,
          padding: '2px 6px',
          display: 'inline-flex',
          alignItems: 'center',
          gap: 2,
        }}
      >
        <ArrowDownOutlined style={{ fontSize: 10 }} /> {item.trendDelta}
      </span>
    );
  }
  return (
    <span
      style={{
        fontSize: 10,
        fontWeight: 700,
        color: '#6B7280',
        background: '#F3F4F6',
        borderRadius: 10,
        padding: '2px 6px',
      }}
    >
      ─
    </span>
  );
}

function CategoryTag({ category }: { category: string }) {
  const cat = CATEGORIES.find((c) => c.key === category) ?? CATEGORIES[4];
  return (
    <span
      style={{
        fontSize: 11,
        fontWeight: 500,
        color: cat.color,
        background: cat.bg,
        borderRadius: 12,
        padding: '2px 8px',
      }}
    >
      {cat.label}
    </span>
  );
}

// 列表单条，独立 React.memo 组件：countdown 每秒 tick 导致 RankingBoard 全量 re-render 时，
// item/dim/isMobile/onClick 引用不变则跳过重渲，20 条列表的渲染开销从 O(N*tick) 降到 O(0)
interface RankingItemProps {
  item: ArticleRanking;
  dim: Dimension;
  isMobile: boolean;
  onClick: (item: ArticleRanking) => void;
}

const RankingItem = React.memo(function RankingItem({
  item,
  dim,
  isMobile,
  onClick,
}: RankingItemProps) {
  return (
    <Link
      href={`/article/${item.articleId}`}
      target='_blank'
      onClick={() => onClick(item)}
      className='no-underline block'
    >
      <div
        className='flex items-center'
        style={{
          padding: isMobile ? '12px 12px' : '16px 20px',
          gap: isMobile ? 10 : 16,
          borderRadius: 8,
          transition: 'background 0.15s',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = '#FAFAFA')}
        onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
      >
        {/* 排名列 */}
        <div
          className='flex flex-col items-center gap-1 shrink-0'
          style={{ width: isMobile ? 44 : 56 }}
        >
          {medal(item.rank) ? (
            <span
              style={{ fontSize: 28, lineHeight: 1, display: 'inline-flex' }}
            >
              {medal(item.rank)}
            </span>
          ) : (
            <span style={{ fontSize: 22, fontWeight: 700, color: '#9CA3AF' }}>
              {item.rank}
            </span>
          )}
          <TrendBadge item={item} />
        </div>

        {/* 内容列 */}
        <div className='flex-1 min-w-0 flex flex-col gap-2'>
          <div
            style={{
              fontSize: isMobile ? 14 : 16,
              fontWeight: 700,
              color: '#1A1A1A',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
            }}
          >
            {item.title}
          </div>
          {/* meta 区：mobile 两行（作者+分类 / 统计三件套），md 单行 */}
          <div className='flex flex-col gap-1 md:flex-row md:items-center md:gap-3'>
            <div className='flex items-center gap-2'>
              <span
                style={{
                  fontSize: 12,
                  fontWeight: 500,
                  color: '#0D9488',
                  maxWidth: 120,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                @{item.author?.name || `用户${item.author?.id}`}
              </span>
              <CategoryTag category={item.category} />
            </div>
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 10,
                fontSize: 12,
                color: '#6B7280',
              }}
            >
              <span className='hidden md:inline' style={{ color: '#9CA3AF' }}>
                ·
              </span>
              <span
                style={{ display: 'inline-flex', alignItems: 'center', gap: 3 }}
              >
                <EyeOutlined /> {formatCount(item.clicks)}
              </span>
              <span
                style={{ display: 'inline-flex', alignItems: 'center', gap: 3 }}
              >
                <LikeOutlined /> {formatCount(item.likes)}
              </span>
              <span
                style={{ display: 'inline-flex', alignItems: 'center', gap: 3 }}
              >
                <StarOutlined /> {formatCount(item.collects)}
              </span>
            </span>
          </div>
          <div className='flex items-center gap-3'>
            <div
              style={{
                flex: 1,
                height: 6,
                background: '#F3F4F6',
                borderRadius: 3,
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  width: `${Math.round((item.scoreRatio || 0) * 100)}%`,
                  height: '100%',
                  background: '#0D9488',
                }}
              />
            </div>
            <span style={{ fontSize: 12, fontWeight: 700, color: '#0D9488' }}>
              {formatScore(item.score, dim)}
            </span>
          </div>
        </div>
      </div>
    </Link>
  );
});

export default function RankingBoard() {
  const { message } = App.useApp();
  const router = useRouter();
  const searchParams = useSearchParams();
  // md 及以上 = 桌面，否则移动端；Pagination simple 等 JS 决策用
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.md;

  // URL 查询参数驱动榜单状态（对齐 article/list.tsx 模式），刷新/返回不丢状态
  // 以 'r' 前缀避免与 /search 页自身的 q/page/size 冲突
  const today = todayDate();
  const dim = (searchParams.get('dim') as Dimension | null) ?? 'hot';
  const cat = searchParams.get('rcat') ?? 'tech';
  const date = searchParams.get('rdate') ?? today;
  const page = Number(searchParams.get('rpage')) || 1;
  const pageSize = Number(searchParams.get('rsize')) || 20;
  const [archiveOpen, setArchiveOpen] = React.useState(false);
  const [archiveDates, setArchiveDates] = React.useState<string[]>([]);
  const [archiveLoading, setArchiveLoading] = React.useState(false);

  const effectiveCat = dim === 'category' ? cat : '';

  // 批量更新 URL 参数（value=null 删除该参数），保留未涉及的参数
  const setParams = React.useCallback(
    (updates: Record<string, string | null>) => {
      const params = new URLSearchParams(searchParams.toString());
      Object.entries(updates).forEach(([k, v]) => {
        if (v === null || v === '') {
          params.delete(k);
        } else {
          params.set(k, v);
        }
      });
      const qs = params.toString();
      router.replace(qs ? `/search?${qs}` : '/search');
    },
    [searchParams, router],
  );

  const { data: res, loading } = useRequest(
    () =>
      rankingApi.pageArticleRanking({
        dimension: dim,
        category: effectiveCat,
        date,
        page,
        pageSize,
      }),
    [dim, effectiveCat, date, page, pageSize],
  );

  const openArchive = async () => {
    setArchiveOpen(true);
    setArchiveLoading(true);
    try {
      const resp = await rankingApi.listArticleRankingArchiveDates();
      setArchiveDates(resp.data?.data ?? []);
    } catch (e) {
      message.error(getErrorMessage(e, '归档列表加载失败'));
    } finally {
      setArchiveLoading(false);
    }
  };
  const selectArchiveDate = (d: string) => {
    // 选今日 → 去掉 rdate 参数保持 URL 简洁；选历史日期 → 写入 rdate
    setParams({ rdate: d === today ? null : d, rpage: null });
    setArchiveOpen(false);
  };
  const archiveNow = async () => {
    try {
      await rankingApi.archiveArticleRanking({ date: today });
      message.success('已触发归档');
      // 刷新归档列表
      const resp = await rankingApi.listArticleRankingArchiveDates();
      setArchiveDates(resp.data?.data ?? []);
    } catch (e) {
      message.error(getErrorMessage(e, '归档失败'));
    }
  };

  // committed = 最近一次成功响应的快照（dim/cat/date/page/pageSize/list/total 全打包）。
  // UI（tab 高亮、列表内容、头部日期、分页器）全部以 committed 为准，请求期间视图不变；
  // 响应到达后 committed 一次性刷新 → 原子切换。请求进行中仅通过顶部进度条给反馈。
  const [committed, setCommitted] = React.useState<{
    dim: Dimension;
    cat: string;
    date: string;
    page: number;
    pageSize: number;
    list: ArticleRanking[];
    total: number;
  }>({ dim, cat, date, page, pageSize, list: [], total: 0 });

  React.useEffect(() => {
    if (!loading && res?.data) {
      setCommitted({
        dim,
        cat,
        date,
        page,
        pageSize,
        list: res.data.list ?? [],
        total: res.data.total ?? 0,
      });
    }
  }, [loading, res, dim, cat, date, page, pageSize]);

  const displayDim = committed.dim;
  const displayCat = committed.cat;
  const displayDate = committed.date;
  const displayPage = committed.page;
  const displayPageSize = committed.pageSize;
  const list = committed.list;
  const total = committed.total;

  // 本次 fetch 成功时记下时间，展示"最后更新"
  const [updatedAt, setUpdatedAt] = React.useState<number | null>(null);
  React.useEffect(() => {
    if (res?.data) {
      setUpdatedAt(Date.now());
    }
  }, [res]);

  // 记住上一次渲染时列表容器的实际高度，loading 中锁住该高度防切 tab 抖动
  // 倒计时每秒刷新
  const [countdown, setCountdown] = React.useState(() =>
    freezeCountdown(new Date()),
  );
  React.useEffect(() => {
    const t = setInterval(
      () => setCountdown(freezeCountdown(new Date())),
      1000,
    );
    return () => clearInterval(t);
  }, []);

  // 用 useCallback 稳定引用，配合 RankingItem 的 React.memo 防止 countdown 每秒 tick 触发 20 条列表全量 re-render；
  // 上报用 displayDim（列表数据所属维度），不是 URL 的 dim（可能是刚点但还没提交的意图维度）
  const handleClick = React.useCallback(
    (item: ArticleRanking) => {
      rankingApi
        .reportArticleRankingClick({
          articleId: item.articleId,
          rank: item.rank,
          dimension: displayDim,
        })
        .catch(() => {
          // 上报失败静默
        });
    },
    [displayDim],
  );

  return (
    <div className='flex flex-col gap-3'>
      {/* Header 条：mobile 垂直堆叠，md 恢复横排 */}
      <div
        className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between md:gap-0'
        style={{
          background: '#FFFFFF',
          borderRadius: 12,
          padding: isMobile ? '14px 16px' : '20px 24px',
        }}
      >
        <div className='flex items-center flex-wrap' style={{ gap: 10 }}>
          <RiseOutlined
            style={{ fontSize: isMobile ? 20 : 24, color: '#0D9488' }}
          />
          <span
            style={{
              fontSize: isMobile ? 16 : 20,
              fontWeight: 700,
              color: '#1A1A1A',
            }}
          >
            {today} 今日榜单
          </span>
          <span
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: '#0D9488',
              background: '#F0FDFA',
              borderRadius: 12,
              padding: '4px 10px',
            }}
          >
            LIVE
          </span>
        </div>
        <div
          className='flex items-center flex-wrap'
          style={{ gap: isMobile ? 8 : 16 }}
        >
          <span style={{ fontSize: 12, fontWeight: 500, color: '#6B7280' }}>
            距次日冻结 {countdown}
          </span>
          <span style={{ fontSize: 12, color: '#9CA3AF' }}>·</span>
          <span style={{ fontSize: 12, fontWeight: 500, color: '#6B7280' }}>
            {updatedAt
              ? `最后更新 ${dayjs(updatedAt).format('HH:mm')}`
              : '加载中…'}
          </span>
          <button
            type='button'
            onClick={openArchive}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 4,
              fontSize: 12,
              fontWeight: 500,
              color: displayDate === today ? '#6B7280' : '#0D9488',
              background: displayDate === today ? '#F5F5F5' : '#F0FDFA',
              border: 'none',
              borderRadius: 8,
              padding: '6px 12px',
              cursor: 'pointer',
            }}
          >
            <HistoryOutlined style={{ fontSize: 14 }} />
            {displayDate === today ? '历史榜单' : `查看 ${displayDate}`}
          </button>
        </div>
      </div>

      {/* Object Tabs（P0 只启用文章）：mobile 溢出横向滚动，避免中文字符被压到换行 */}
      <div
        className='flex overflow-x-auto'
        style={{
          background: '#FFFFFF',
          borderRadius: 12,
          padding: isMobile ? '0 12px' : '0 24px',
          gap: isMobile ? 16 : 24,
          scrollbarWidth: 'none',
        }}
      >
        {OBJECTS.map((o) => {
          const active = o.key === 'article';
          return (
            <div
              key={o.key}
              className='shrink-0'
              style={{
                padding: active ? '16px 0 14px 0' : '16px 0',
                borderBottom: active ? '2px solid #0D9488' : 'none',
                opacity: o.enabled ? 1 : 0.4,
                cursor: o.enabled ? 'pointer' : 'not-allowed',
              }}
              title={o.enabled ? undefined : 'P0 暂未开放'}
            >
              <span
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  fontSize: isMobile ? 14 : 15,
                  fontWeight: active ? 700 : 500,
                  color: active ? '#0D9488' : '#6B7280',
                  whiteSpace: 'nowrap',
                }}
              >
                {o.icon}
                {o.label}
              </span>
            </div>
          );
        })}
      </div>

      {/* Dimension Tabs：mobile flex-wrap 防止横向溢出 */}
      <div
        className='flex items-center flex-wrap gap-2'
        style={{ padding: '4px 0' }}
      >
        {DIMENSIONS.map((d) => {
          const active = d.key === displayDim;
          return (
            <button
              key={d.key}
              onClick={() =>
                setParams({ dim: d.key === 'hot' ? null : d.key, rpage: null })
              }
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                fontSize: 13,
                fontWeight: active ? 600 : 500,
                color: active ? '#FFFFFF' : '#1A1A1A',
                background: active ? '#0D9488' : '#FFFFFF',
                border: active ? 'none' : '1px solid #E5E7EB',
                borderRadius: 8,
                padding: isMobile ? '6px 12px' : '8px 16px',
                cursor: 'pointer',
              }}
            >
              {d.icon}
              {d.label}
            </button>
          );
        })}
      </div>

      {/* 分区选择器 */}
      {displayDim === 'category' && (
        <div className='flex items-center flex-wrap gap-2'>
          {CATEGORIES.map((c) => {
            const active = c.key === displayCat;
            return (
              <button
                key={c.key}
                onClick={() =>
                  setParams({
                    rcat: c.key === 'tech' ? null : c.key,
                    rpage: null,
                  })
                }
                style={{
                  fontSize: 12,
                  fontWeight: 500,
                  color: active ? '#0D9488' : '#6B7280',
                  background: active ? '#F0FDFA' : '#FFFFFF',
                  border: `1px solid ${active ? '#0D9488' : '#E5E7EB'}`,
                  borderRadius: 8,
                  padding: '6px 12px',
                  cursor: 'pointer',
                }}
              >
                {c.label}
              </button>
            );
          })}
        </div>
      )}

      {/* 列表：committed 快照 + 顶部进度条反馈 loading；切 tab 请求期间列表不变，响应到达时原子切换 */}
      <div
        style={{
          background: '#FFFFFF',
          borderRadius: 12,
          padding: '8px',
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        {loading && <div className='top-progress-bar' />}
        <div>
          {loading && list.length === 0 && (
            <Skeleton active style={{ padding: 16 }} />
          )}
          {!loading && list.length === 0 && (
            <Empty description='榜单生成中，一会再来' style={{ padding: 32 }} />
          )}
          {list.length > 0 &&
            list.map((item) => (
              <RankingItem
                key={item.articleId}
                item={item}
                dim={displayDim}
                isMobile={isMobile}
                onClick={handleClick}
              />
            ))}
        </div>
      </div>

      {/* 分页：对齐系统标准（article/list.tsx & feed.tsx），mobile 额外 simple 模式 */}
      {total > displayPageSize && (
        <div className='flex justify-center' style={{ padding: '12px 0' }}>
          <Pagination
            current={displayPage}
            pageSize={displayPageSize}
            total={total}
            showTotal={(t) => `共 ${t} 条`}
            showSizeChanger
            showQuickJumper
            pageSizeOptions={['10', '20', '50']}
            size='small'
            simple={isMobile}
            onChange={(p, s) =>
              setParams({
                rpage: p === 1 ? null : String(p),
                rsize: s === 20 ? null : String(s),
              })
            }
          />
        </div>
      )}

      {/* 历史榜单 Drawer */}
      <Drawer
        title='历史榜单'
        placement='right'
        open={archiveOpen}
        onClose={() => setArchiveOpen(false)}
        width={360}
        extra={
          <button
            type='button'
            onClick={archiveNow}
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: '#FFFFFF',
              background: '#0D9488',
              border: 'none',
              borderRadius: 6,
              padding: '4px 10px',
              cursor: 'pointer',
            }}
          >
            归档今日
          </button>
        }
      >
        {archiveLoading ? (
          <Skeleton active />
        ) : archiveDates.length === 0 ? (
          <Empty description='还没有归档日期' />
        ) : (
          <List
            dataSource={[today, ...archiveDates.filter((d) => d !== today)]}
            renderItem={(d) => (
              <List.Item
                onClick={() => selectArchiveDate(d)}
                style={{
                  cursor: 'pointer',
                  background: d === date ? '#F0FDFA' : undefined,
                  padding: '12px 16px',
                  borderRadius: 8,
                }}
              >
                <span
                  style={{
                    fontSize: 14,
                    fontWeight: d === date ? 700 : 500,
                    color: d === date ? '#0D9488' : '#1A1A1A',
                  }}
                >
                  {d} {d === today && '（今日）'}
                </span>
              </List.Item>
            )}
          />
        )}
      </Drawer>
    </div>
  );
}
