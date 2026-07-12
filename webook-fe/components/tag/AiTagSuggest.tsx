'use client';

import { PlusOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { App, Spin, theme } from 'antd';
import { useState } from 'react';

import { recommendTags } from '@/api/tag';
import { MAX_TAGS } from '@/constants/tag';
import { getErrorMessage } from '@/utils/apiError';

interface AiTagSuggestProps {
  getContext: () => { title: string; content: string };
  selected: string[];
  onAdd: (tag: string) => void;
}

// AI 推荐标签：基于标题+正文（embedding + ES kNN 相似文章标签聚合）产出候选，点击加入。
export function AiTagSuggest({
  getContext,
  selected,
  onAdd,
}: AiTagSuggestProps) {
  const { message } = App.useApp();
  const { token } = theme.useToken();
  const [loading, setLoading] = useState(false);
  const [candidates, setCandidates] = useState<string[]>([]);

  const handleRecommend = async () => {
    const { title, content } = getContext();
    if (!title.trim() || !content.trim()) {
      message.warning('请先输入标题和内容');
      return;
    }
    setLoading(true);
    try {
      const res = await recommendTags({ title, content });
      setCandidates((res.data.data ?? []).map((t) => t.name));
    } catch (e) {
      message.error(getErrorMessage(e, 'AI 推荐失败'));
    } finally {
      setLoading(false);
    }
  };

  const remaining = candidates.filter((c) => !selected.includes(c));
  const full = selected.length >= MAX_TAGS;

  return (
    <div>
      <button
        type='button'
        onClick={handleRecommend}
        disabled={loading}
        className='inline-flex items-center gap-1 border-0 bg-transparent p-0 text-sm'
        style={{
          color: token.colorInfo,
          cursor: loading ? 'default' : 'pointer',
        }}
      >
        {loading ? <Spin size='small' /> : <ThunderboltOutlined />}
        AI 推荐标签（基于正文语义）
      </button>
      {remaining.length > 0 && (
        <div className='mt-2 flex flex-wrap gap-2'>
          {remaining.map((c) => (
            <button
              key={c}
              type='button'
              disabled={full}
              onClick={() => onAdd(c)}
              className='inline-flex items-center gap-1 px-2 py-0.5 text-sm'
              style={{
                color: token.colorInfo,
                border: `1px solid ${token.colorInfo}`,
                borderRadius: 12,
                background: 'transparent',
                cursor: full ? 'not-allowed' : 'pointer',
                opacity: full ? 0.5 : 1,
              }}
            >
              <PlusOutlined /> {c}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
