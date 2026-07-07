'use client';

import { App } from 'antd';
import dayjs from 'dayjs';
import { Heart, Reply, Trash2 } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React from 'react';

import { UserHoverCard } from '@/components/relation/UserHoverCard';
import type { Comment } from '@/types';

// 头像配色循环（对齐原型 $primary/$av-amber/$av-green/$av-indigo）
const AVATAR_COLORS = ['#0D9488', '#D97706', '#22C55E', '#6366F1'];

function avatarColor(uid: number) {
  return AVATAR_COLORS[Math.abs(uid) % AVATAR_COLORS.length];
}

function initial(name: string) {
  return name?.trim()?.[0]?.toUpperCase() ?? '?';
}

// relativeTime 轻量相对时间（避免引入 dayjs relativeTime 插件全局配置）
function relativeTime(ms: number): string {
  const diff = Date.now() - ms;
  const minute = 60_000;
  const hour = 3_600_000;
  const day = 86_400_000;
  if (diff < minute) {
    return '刚刚';
  }
  if (diff < hour) {
    return `${Math.floor(diff / minute)} 分钟前`;
  }
  if (diff < day) {
    return `${Math.floor(diff / hour)} 小时前`;
  }
  if (diff < 7 * day) {
    return `${Math.floor(diff / day)} 天前`;
  }
  return dayjs(ms).format('YYYY-MM-DD');
}

const ACTION_BTN =
  'flex items-center gap-1.5 bg-transparent border-none cursor-pointer p-0';

interface CommentItemProps {
  comment: Comment;
  isReply?: boolean;
  currentUid: number | null;
  onLike: (c: Comment) => void;
  onReply: (c: Comment) => void;
  onDelete: (c: Comment) => void;
}

// CommentItem 单条评论：头像 + 元信息 + 正文 + 操作（赞/回复/删除）。线程结构由 CommentSection 编排
export function CommentItem({
  comment,
  isReply,
  currentUid,
  onLike,
  onReply,
  onDelete,
}: CommentItemProps) {
  const { modal } = App.useApp();
  const router = useRouter();
  const size = isReply ? 30 : 36;
  const goProfile = () => router.push(`/user/${comment.user.id}`);
  const isMine = currentUid !== null && comment.user.id === currentUid;

  // 删除走全局 Modal.confirm（项目约定）；避免每条评论挂一个 Popconfirm
  // 触发 antd CSS-in-JS「unmount 后注册清理」告警
  const confirmDelete = () =>
    modal.confirm({
      title: '删除这条评论？',
      content:
        !isReply && comment.replyCnt > 0
          ? `删除后，该评论下的 ${comment.replyCnt} 条回复也将一并删除`
          : undefined,
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: () => onDelete(comment),
    });

  return (
    <div className='flex gap-2.5'>
      <UserHoverCard userId={comment.user.id} self={isMine}>
        <div
          onClick={goProfile}
          className='flex shrink-0 cursor-pointer items-center justify-center'
          style={{
            width: size,
            height: size,
            borderRadius: '50%',
            background: avatarColor(comment.user.id),
          }}
        >
          <span
            style={{
              color: '#FFFFFF',
              fontSize: isReply ? 12 : 14,
              fontWeight: 600,
            }}
          >
            {initial(comment.user.name)}
          </span>
        </div>
      </UserHoverCard>

      <div className='flex flex-col gap-1.5 flex-1 min-w-0'>
        {/* 元信息 */}
        <div className='flex items-center gap-2'>
          <UserHoverCard userId={comment.user.id} self={isMine}>
            <span
              onClick={goProfile}
              className='cursor-pointer hover:text-[#0D9488]'
              style={{ fontSize: 13, fontWeight: 600, color: '#1A1A1A' }}
            >
              {comment.user.name}
            </span>
          </UserHoverCard>
          {isMine && (
            <span
              style={{
                fontSize: 11,
                fontWeight: 600,
                color: '#0D9488',
                background: '#E6F4F2',
                borderRadius: 4,
                padding: '0 5px',
              }}
            >
              我
            </span>
          )}
          <span style={{ fontSize: 12, color: '#9CA3AF' }}>
            · {relativeTime(comment.createdAt)}
          </span>
        </div>

        {/* 正文 */}
        <div
          style={{
            fontSize: 14,
            color: '#1A1A1A',
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {comment.content}
        </div>

        {/* 操作栏 */}
        <div className='flex items-center' style={{ gap: 18 }}>
          <button
            type='button'
            className={ACTION_BTN}
            onClick={() => onLike(comment)}
          >
            <Heart
              size={15}
              fill={comment.liked ? '#EF4444' : 'none'}
              color={comment.liked ? '#EF4444' : '#9CA3AF'}
            />
            <span
              style={{
                fontSize: 13,
                fontWeight: 600,
                color: comment.liked ? '#EF4444' : '#6B7280',
              }}
            >
              {comment.likeCnt}
            </span>
          </button>

          <button
            type='button'
            className={ACTION_BTN}
            onClick={() => onReply(comment)}
          >
            <Reply size={15} color='#9CA3AF' />
            <span style={{ fontSize: 13, color: '#6B7280' }}>回复</span>
          </button>

          {isMine && (
            <button
              type='button'
              className={ACTION_BTN}
              onClick={confirmDelete}
            >
              <Trash2 size={15} color='#9CA3AF' />
              <span style={{ fontSize: 13, color: '#6B7280' }}>删除</span>
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
