'use client';

import { MoreOutlined } from '@ant-design/icons';
import { App, Button, Card, Dropdown, Empty, Tabs } from 'antd';
import { Ban, Flag, Mail, RotateCcw } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React from 'react';

import { pageAuthorArticles } from '@/api/article';
import { block, getRelationStat, unblock } from '@/api/relation';
import { findProfile, findUserInfo } from '@/api/user';
import { Loading } from '@/components/common/Loading';
import { PublicHeader } from '@/components/layout/PublicHeader';
import { FollowButton } from '@/components/relation/FollowButton';
import { PALETTE } from '@/constants/theme';
import type { ReaderArticle, RelationStat, UserInfo } from '@/types';
import { getErrorMessage } from '@/utils/apiError';
import { formatCount, joinedFor, relativeTime } from '@/utils/format';
import { tokenUtil } from '@/utils/token';

const PAGE_SIZE = 10;

function Count({ n, label }: { n: number; label: string }) {
  return (
    <div className='flex items-baseline gap-1.5'>
      <span className='text-base font-bold text-ink'>{formatCount(n)}</span>
      <span className='text-sm text-muted'>{label}</span>
    </div>
  );
}

export default function UserDetailPage({ userId }: { userId: string }) {
  const id = Number(userId);
  const router = useRouter();
  const { message, modal } = App.useApp();

  const [info, setInfo] = React.useState<UserInfo | null>(null);
  const [stat, setStat] = React.useState<RelationStat | null>(null);
  const [articles, setArticles] = React.useState<ReaderArticle[]>([]);
  const [likedTotal, setLikedTotal] = React.useState(0);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [loading, setLoading] = React.useState(true);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [notFound, setNotFound] = React.useState(false);
  const [myUid, setMyUid] = React.useState<number | null>(null);

  const isSelf = myUid !== null && myUid === id;

  React.useEffect(() => {
    if (tokenUtil.hasToken()) {
      findProfile()
        .then((r) => setMyUid(r.data.id))
        .catch(() => {});
    }
  }, []);

  const refreshStat = React.useCallback(async () => {
    try {
      const r = await getRelationStat(id);
      setStat(r.data.data);
    } catch {
      /* 关系服务降级：保留旧值 */
    }
  }, [id]);

  React.useEffect(() => {
    if (!id) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    setNotFound(false);
    Promise.all([
      findUserInfo(id)
        .then((r) => r.data.data)
        .catch(() => null),
      getRelationStat(id)
        .then((r) => r.data.data)
        .catch(() => null),
      pageAuthorArticles({ authorId: id, page: 1, pageSize: PAGE_SIZE })
        .then((r) => r.data.data)
        .catch(() => null),
    ]).then(([infoRes, statRes, artRes]) => {
      if (cancelled) {
        return;
      }
      if (!infoRes) {
        setNotFound(true);
        setLoading(false);
        return;
      }
      setInfo(infoRes);
      setStat(statRes);
      if (artRes) {
        setArticles(artRes.list ?? []);
        setTotal(artRes.total ?? 0);
        setLikedTotal(artRes.likedTotal ?? 0);
        setPage(1);
      }
      setLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [id]);

  const loadMore = async () => {
    const next = page + 1;
    setLoadingMore(true);
    try {
      const r = await pageAuthorArticles({
        authorId: id,
        page: next,
        pageSize: PAGE_SIZE,
      });
      setArticles((prev) => [...prev, ...(r.data.data.list ?? [])]);
      setPage(next);
    } catch (e) {
      message.error(getErrorMessage(e, '加载失败'));
    } finally {
      setLoadingMore(false);
    }
  };

  const doBlock = () => {
    modal.confirm({
      title: `拉黑 ${info?.nickname || '该用户'}？`,
      content:
        '拉黑后将解除你与 TA 的相互关注；TA 将无法关注你、给你发私信或评论你的内容。可随时在黑名单中取消。',
      okText: '确定拉黑',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await block(id);
          message.success('已拉黑');
          await refreshStat();
        } catch (e) {
          message.error(getErrorMessage(e, '拉黑失败'));
          throw e; // 保持弹窗打开
        }
      },
    });
  };

  const doUnblock = async () => {
    try {
      await unblock(id);
      message.success('已取消拉黑');
      await refreshStat();
    } catch (e) {
      message.error(getErrorMessage(e, '取消拉黑失败'));
    }
  };

  const menuItems = stat?.isBlocked
    ? [
        {
          key: 'unblock',
          label: '取消拉黑',
          icon: <RotateCcw size={14} />,
          onClick: doUnblock,
        },
      ]
    : [
        {
          key: 'block',
          label: '拉黑',
          danger: true,
          icon: <Ban size={14} />,
          onClick: doBlock,
        },
        {
          key: 'report',
          label: '举报',
          icon: <Flag size={14} />,
          onClick: () => message.info('举报已提交，感谢反馈'),
        },
      ];

  let body: React.ReactNode;
  if (loading) {
    body = <Loading />;
  } else if (notFound || !info) {
    body = <Empty description='用户不存在' className='mt-16' />;
  } else {
    body = (
      <div className='flex flex-col gap-3'>
        {/* 头部（对齐 relation.pen ProfileCard：row1[头像|信息|操作] + 独立计数排） */}
        <Card>
          <div className='flex flex-col gap-[18px]'>
            {/* row1 */}
            <div className='flex flex-col gap-4 md:flex-row md:items-start'>
              <div
                className='flex h-20 w-20 shrink-0 items-center justify-center self-center rounded-full text-3xl font-bold md:self-start'
                style={{
                  background: PALETTE.tealSurface,
                  color: PALETTE.primary,
                }}
              >
                {info.nickname?.[0]?.toUpperCase() || '?'}
              </div>

              <div className='flex min-w-0 flex-1 flex-col gap-1.5'>
                <div className='flex items-center justify-center gap-2.5 md:justify-start'>
                  <span className='text-xl font-bold leading-tight text-ink'>
                    {info.nickname || `用户 #${id}`}
                  </span>
                  {stat?.isMutual && (
                    <span
                      className='rounded-full px-2.5 py-1 text-[11px] font-medium leading-none'
                      style={{
                        background: PALETTE.tealSurface,
                        color: PALETTE.primary,
                      }}
                    >
                      互相关注
                    </span>
                  )}
                </div>
                <div className='text-center text-[13px] text-subtle md:text-left'>
                  {joinedFor(info.createdAt)}
                </div>
                {info.aboutMe && (
                  <div className='text-center text-sm text-muted md:text-left'>
                    {info.aboutMe}
                  </div>
                )}
              </div>

              {/* 操作区 */}
              <div className='flex shrink-0 items-center justify-center gap-2.5'>
                {isSelf ? (
                  <Button onClick={() => router.push('/user/profile')}>
                    编辑资料
                  </Button>
                ) : (
                  <>
                    <FollowButton
                      targetId={id}
                      isFollowing={stat?.isFollowing ?? false}
                      isMutual={stat?.isMutual ?? false}
                      isBlocked={stat?.isBlocked}
                      isBlockedBy={stat?.isBlockedBy}
                      onChanged={refreshStat}
                    />
                    <Button
                      icon={<Mail size={15} />}
                      onClick={() => message.info('私信功能开发中，敬请期待')}
                    >
                      私信
                    </Button>
                    <Dropdown
                      menu={{ items: menuItems }}
                      trigger={['click']}
                      placement='bottomRight'
                    >
                      <Button icon={<MoreOutlined />} />
                    </Dropdown>
                  </>
                )}
              </div>
            </div>

            {/* 计数：独立一排（对齐原型，不嵌在信息列内） */}
            <div className='flex justify-center gap-7 md:justify-start'>
              <Count n={stat?.followeeCnt ?? 0} label='关注' />
              <Count n={stat?.followerCnt ?? 0} label='粉丝' />
              <Count n={likedTotal} label='获赞' />
            </div>
          </div>
        </Card>

        {/* TA 的文章 */}
        <Card styles={{ body: { paddingTop: 8 } }}>
          <Tabs
            items={[
              {
                key: 'articles',
                label: `TA 的文章 ${total}`,
                children:
                  articles.length === 0 ? (
                    <Empty description='还没有发布文章' className='py-10' />
                  ) : (
                    <div className='flex flex-col gap-3'>
                      {articles.map((a) => (
                        <a
                          key={a.id}
                          href={`/article/${a.id}`}
                          target='_blank'
                          rel='noreferrer'
                          className='block rounded-xl border border-line p-4 transition-colors hover:bg-surface-hover'
                        >
                          <div className='font-bold text-ink'>{a.title}</div>
                          <div className='mt-1.5 text-xs text-subtle'>
                            {relativeTime(a.createdAt)} · 点赞{' '}
                            {formatCount(a.likeCnt)} · 评论{' '}
                            {formatCount(a.commentCnt)}
                          </div>
                        </a>
                      ))}
                      {articles.length < total && (
                        <div className='flex justify-center py-2'>
                          <Button onClick={loadMore} loading={loadingMore}>
                            加载更多
                          </Button>
                        </div>
                      )}
                    </div>
                  ),
              },
            ]}
          />
        </Card>
      </div>
    );
  }

  return (
    <div className='flex h-screen flex-col overflow-hidden bg-page'>
      <PublicHeader />
      <div className='flex-1 overflow-y-auto'>
        <div className='mx-auto max-w-3xl px-4 py-6'>{body}</div>
      </div>
    </div>
  );
}
