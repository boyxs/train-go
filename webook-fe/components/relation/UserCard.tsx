'use client';

import React from 'react';

// 用户卡（列表项），精确对齐 relation.pen「UserCard」(dEV64)：
// radius12 · border · padding16 · gap14 · 头像 48 pill · 名 15/600 · bio 13 tertiary · 互关 pill
const AVATAR_PALETTE = [
  { bg: '#F0FDFA', fg: '#0D9488' },
  { bg: '#EEF2FF', fg: '#6366F1' },
  { bg: '#FFFBEB', fg: '#D97706' },
  { bg: '#F0FDF4', fg: '#22C55E' },
  { bg: '#FEF2F2', fg: '#EF4444' },
];

interface UserCardProps {
  id: number;
  name: string;
  bio?: string;
  sub?: string; // 覆盖 bio 行（如黑名单「已拉黑 · 5 天前」）
  mutual?: boolean; // 显示「互相关注」标记
  muted?: boolean; // 头像置灰（黑名单）
  right?: React.ReactNode; // 右侧操作区
  onClick?: () => void; // 点卡片跳主页
}

export function UserCard({
  id,
  name,
  bio,
  sub,
  mutual = false,
  muted = false,
  right,
  onClick,
}: UserCardProps) {
  const initial = name?.[0]?.toUpperCase() || '?';
  const color = muted
    ? { bg: '#F3F4F6', fg: '#9CA3AF' }
    : AVATAR_PALETTE[Math.abs(id) % AVATAR_PALETTE.length];
  const subText = sub ?? bio ?? '';

  return (
    <div
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onClick();
              }
            }
          : undefined
      }
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        padding: 16,
        borderRadius: 12,
        border: '1px solid #E5E7EB',
        background: '#FFFFFF',
        cursor: onClick ? 'pointer' : 'default',
      }}
    >
      {/* 头像 */}
      <div
        style={{
          width: 48,
          height: 48,
          flexShrink: 0,
          borderRadius: '50%',
          background: color.bg,
          color: color.fg,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 18,
          fontWeight: 700,
        }}
      >
        {initial}
      </div>

      {/* 信息 */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontSize: 15, fontWeight: 600, color: '#1A1A1A' }}>
            {name || `用户 #${id}`}
          </span>
          {mutual && (
            <span
              style={{
                fontSize: 11,
                fontWeight: 500,
                color: '#0D9488',
                background: '#F0FDFA',
                borderRadius: 999,
                padding: '4px 10px',
                lineHeight: 1,
                whiteSpace: 'nowrap',
              }}
            >
              互相关注
            </span>
          )}
        </div>
        {subText && (
          <div
            style={{
              marginTop: 5,
              fontSize: 13,
              color: '#9CA3AF',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {subText}
          </div>
        )}
      </div>

      {/* 右侧操作：阻止冒泡，避免点按钮触发卡片跳转 */}
      {right && (
        <div onClick={(e) => e.stopPropagation()} style={{ flexShrink: 0 }}>
          {right}
        </div>
      )}
    </div>
  );
}
