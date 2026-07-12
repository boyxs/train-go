'use client';

import { CheckOutlined } from '@ant-design/icons';
import { theme } from 'antd';

import { PALETTE } from '@/constants/theme';
import type { TagFacet } from '@/types';
import { formatCount } from '@/utils/format';

interface FacetBarProps {
  facets: TagFacet[];
  selected: string[]; // 选中的 slug（多选 AND）
  onToggle: (slug: string) => void;
  onClear: () => void;
}

// 搜索结果标签 facet：选中 teal 实心 + ✓，未选描边；点击切换，「清除筛选」清空。
export function FacetBar({
  facets,
  selected,
  onToggle,
  onClear,
}: FacetBarProps) {
  const { token } = theme.useToken();
  if (facets.length === 0) {
    return null;
  }
  return (
    <div className='flex flex-wrap items-center gap-2'>
      <span className='text-sm text-gray-400'>按标签筛选</span>
      {facets.map((f) => {
        const on = selected.includes(f.slug);
        return (
          <button
            key={f.slug}
            type='button'
            onClick={() => onToggle(f.slug)}
            className='inline-flex items-center gap-1 px-3 py-1 text-sm'
            style={{
              borderRadius: 12,
              border: `1px solid ${on ? token.colorPrimary : token.colorBorder}`,
              background: on ? token.colorPrimary : 'transparent',
              color: on ? PALETTE.surface : token.colorText,
              cursor: 'pointer',
            }}
          >
            {on && <CheckOutlined />}
            {f.name}
            <span
              style={{
                color: on ? 'rgba(255,255,255,0.75)' : token.colorTextTertiary,
              }}
            >
              {formatCount(f.count)}
            </span>
          </button>
        );
      })}
      {selected.length > 0 && (
        <button
          type='button'
          onClick={onClear}
          className='border-0 bg-transparent text-sm'
          style={{ color: token.colorPrimary, cursor: 'pointer' }}
        >
          清除筛选
        </button>
      )}
    </div>
  );
}
