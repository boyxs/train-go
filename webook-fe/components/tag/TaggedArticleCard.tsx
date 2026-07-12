'use client';

import { EyeOutlined, LikeOutlined, StarOutlined } from '@ant-design/icons';
import { theme, Typography } from 'antd';
import Link from 'next/link';

import { PALETTE } from '@/constants/theme';
import type { TaggedArticle } from '@/types';
import { formatCount } from '@/utils/format';

const { Text } = Typography;

// 标签页 / 搜索结果共用的文章卡：标题 + 摘要 + 分区/标签 chips + 作者与互动计数。
export function TaggedArticleCard({
  article,
  activeTags = [],
}: {
  article: TaggedArticle;
  activeTags?: string[]; // 命中当前搜索筛选的标签 slug → 卡内 teal 实心高亮
}) {
  const { token } = theme.useToken();
  const pill = 'inline-block px-2 py-0.5 text-xs';

  return (
    <div className='rounded-xl bg-white px-6 py-5 transition-colors hover:bg-gray-50'>
      <Link
        href={`/article/${article.id}`}
        target='_blank'
        className='no-underline'
      >
        <Text
          strong
          style={{ fontSize: 16 }}
          className='transition-colors hover:text-primary'
        >
          {article.title}
        </Text>
      </Link>

      {article.abstract && (
        <div className='mt-2'>
          <Text type='secondary' style={{ fontSize: 14, lineHeight: '1.5' }}>
            {article.abstract}
          </Text>
        </div>
      )}

      {(article.category || article.tags.length > 0) && (
        <div className='mt-3 flex flex-wrap items-center gap-2'>
          {article.category && (
            <span
              className={pill}
              style={{
                borderRadius: 12,
                background: token.colorInfoBg,
                color: token.colorInfo,
              }}
            >
              {article.category}
            </span>
          )}
          {article.tags.map((t) => {
            const on = activeTags.includes(t.slug);
            return (
              <Link
                key={t.slug}
                href={`/tag/${encodeURIComponent(t.slug)}`}
                className='no-underline'
              >
                <span
                  className={pill}
                  style={{
                    borderRadius: 12,
                    // 命中筛选 = teal 实心高亮（PALETTE.primary）；默认 = 浅 teal（PALETTE.tealSurface）
                    background: on ? PALETTE.primary : PALETTE.tealSurface,
                    color: on ? PALETTE.surface : PALETTE.primary,
                  }}
                >
                  {t.name}
                </span>
              </Link>
            );
          })}
        </div>
      )}

      <div className='mt-3 flex items-center gap-4 text-xs text-gray-400'>
        <Link
          href={`/user/${article.author.id}`}
          className='no-underline'
          style={{ color: token.colorPrimary }}
        >
          @{article.author.name}
        </Link>
        <span>
          <EyeOutlined /> {formatCount(article.readCnt)}
        </span>
        <span>
          <LikeOutlined /> {formatCount(article.likeCnt)}
        </span>
        <span>
          <StarOutlined /> {formatCount(article.collectCnt)}
        </span>
      </div>
    </div>
  );
}
