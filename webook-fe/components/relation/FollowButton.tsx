'use client';

import { App } from 'antd';
import { Ban, Check, Loader2, Plus, Users } from 'lucide-react';
import React from 'react';

import { follow, unfollow } from '@/api/relation';
import { getErrorMessage } from '@/utils/apiError';
import { tokenUtil } from '@/utils/token';

// 关注按钮 4 态，精确对齐 relation.pen「关注按钮 · 4 态」：
// padding[8,20] gap6 radius8 · icon 16 lucide · 文字 14/600
type FollowState = 'not-following' | 'following' | 'mutual' | 'blocked';

const STYLES: Record<
  FollowState,
  { bg: string; fg: string; border: string; label: string }
> = {
  'not-following': {
    bg: '#0D9488',
    fg: '#FFFFFF',
    border: '#0D9488',
    label: '关注',
  },
  following: {
    bg: '#FFFFFF',
    fg: '#6B7280',
    border: '#E5E7EB',
    label: '已关注',
  },
  mutual: {
    bg: '#F0FDFA',
    fg: '#0D9488',
    border: '#99F6E4',
    label: '互相关注',
  },
  blocked: {
    bg: '#F5F5F5',
    fg: '#D1D5DB',
    border: '#E5E7EB',
    label: '无法关注',
  },
};

const ICON: Record<FollowState, React.ComponentType<{ size?: number }>> = {
  'not-following': Plus,
  following: Check,
  mutual: Users,
  blocked: Ban,
};

interface FollowButtonProps {
  targetId: number;
  isFollowing: boolean;
  isMutual: boolean;
  isBlockedBy?: boolean; // 对方拉黑了我 → 无法关注（禁用）
  isBlocked?: boolean; // 我拉黑了对方 → 点击提示先取消拉黑
  // 关注成功后是否构成互关（调用方已知对方是否关注我时传入，如粉丝列表「回关」= true）
  mutualOnFollow?: boolean;
  followLabel?: string; // 未关注态标签覆盖（如粉丝列表「回关」）
  onChanged?: (next: { isFollowing: boolean; isMutual: boolean }) => void;
}

export function FollowButton({
  targetId,
  isFollowing,
  isMutual,
  isBlockedBy = false,
  isBlocked = false,
  mutualOnFollow = false,
  followLabel,
  onChanged,
}: FollowButtonProps) {
  const { message } = App.useApp();
  const [following, setFollowing] = React.useState(isFollowing);
  const [mutual, setMutual] = React.useState(isMutual);
  const [loading, setLoading] = React.useState(false);

  // 外部数据刷新时同步内部乐观态
  React.useEffect(() => {
    setFollowing(isFollowing);
    setMutual(isMutual);
  }, [isFollowing, isMutual]);

  const state: FollowState = isBlockedBy
    ? 'blocked'
    : !following
      ? 'not-following'
      : mutual
        ? 'mutual'
        : 'following';

  const handleClick = async () => {
    if (isBlockedBy || loading) {
      return;
    }
    if (!tokenUtil.hasToken()) {
      message.warning('请先登录');
      return;
    }
    if (isBlocked) {
      message.warning('已拉黑对方，请先取消拉黑');
      return;
    }
    const prev = { following, mutual };
    const next = following
      ? { isFollowing: false, isMutual: false }
      : { isFollowing: true, isMutual: mutualOnFollow };
    // 乐观更新
    setFollowing(next.isFollowing);
    setMutual(next.isMutual);
    setLoading(true);
    try {
      if (prev.following) {
        await unfollow(targetId);
      } else {
        await follow(targetId);
      }
      onChanged?.(next);
    } catch (e) {
      // 回滚
      setFollowing(prev.following);
      setMutual(prev.mutual);
      message.error(
        getErrorMessage(e, prev.following ? '取关失败' : '关注失败'),
      );
    } finally {
      setLoading(false);
    }
  };

  const s = STYLES[state];
  const Icon = ICON[state];
  const disabled = isBlockedBy;
  const label =
    state === 'not-following' && followLabel ? followLabel : s.label;

  return (
    <button
      type='button'
      onClick={handleClick}
      disabled={disabled || loading}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: '8px 20px',
        borderRadius: 8,
        border: `1px solid ${s.border}`,
        background: s.bg,
        color: s.fg,
        fontSize: 14,
        fontWeight: 600,
        lineHeight: 1,
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: loading ? 0.7 : 1,
        transition: 'opacity 0.15s',
        whiteSpace: 'nowrap',
      }}
    >
      {loading ? (
        <Loader2 size={16} className='animate-spin' />
      ) : (
        <Icon size={16} />
      )}
      {label}
    </button>
  );
}
