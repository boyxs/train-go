'use client';

import { App } from 'antd';
import { Check, Loader2, Plus } from 'lucide-react';
import React from 'react';

import { followTag, unfollowTag } from '@/api/tag';
import { PALETTE } from '@/constants/theme';
import { getErrorMessage } from '@/utils/apiError';
import { tokenUtil } from '@/utils/token';

// 标签关注按钮 2 态：未关注 = teal 实心，已关注 = 白底灰字描边。乐观切换 + 登录门控 + 失败回滚。
interface TagFollowButtonProps {
  slug: string;
  isFollowing: boolean;
  // 翻转成功后回传服务端最新关注态 + 关注数（供页面更新「N 人关注」）
  onChanged?: (next: { isFollowing: boolean; followCount: number }) => void;
}

export function TagFollowButton({
  slug,
  isFollowing,
  onChanged,
}: TagFollowButtonProps) {
  const { message } = App.useApp();
  const [following, setFollowing] = React.useState(isFollowing);
  const [loading, setLoading] = React.useState(false);

  // 外部数据刷新时同步内部乐观态
  React.useEffect(() => {
    setFollowing(isFollowing);
  }, [isFollowing]);

  const handleClick = async () => {
    if (loading) {
      return;
    }
    if (!tokenUtil.hasToken()) {
      message.warning('请先登录');
      return;
    }
    const prev = following;
    setFollowing(!prev); // 乐观更新
    setLoading(true);
    try {
      const res = prev ? await unfollowTag(slug) : await followTag(slug);
      const data = res.data.data;
      setFollowing(data.isFollowing);
      onChanged?.({
        isFollowing: data.isFollowing,
        followCount: data.followCount,
      });
    } catch (e) {
      setFollowing(prev); // 回滚
      message.error(getErrorMessage(e, prev ? '取关失败' : '关注失败'));
    } finally {
      setLoading(false);
    }
  };

  const style = following
    ? {
        background: PALETTE.surface,
        color: PALETTE.muted,
        border: `1px solid ${PALETTE.line}`,
      }
    : {
        background: PALETTE.primary,
        color: PALETTE.surface,
        border: `1px solid ${PALETTE.primary}`,
      };
  const Icon = following ? Check : Plus;

  return (
    <button
      type='button'
      onClick={handleClick}
      disabled={loading}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: '8px 20px',
        borderRadius: 8,
        fontSize: 14,
        fontWeight: 600,
        lineHeight: 1,
        cursor: loading ? 'not-allowed' : 'pointer',
        opacity: loading ? 0.7 : 1,
        transition: 'opacity 0.15s',
        whiteSpace: 'nowrap',
        ...style,
      }}
    >
      {loading ? (
        <Loader2 size={16} className='animate-spin' />
      ) : (
        <Icon size={16} />
      )}
      {following ? '已关注' : '关注'}
    </button>
  );
}
