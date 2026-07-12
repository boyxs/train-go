'use client';

import { CloseOutlined, PlusOutlined } from '@ant-design/icons';
import { App, Select, theme } from 'antd';
import type { SelectProps } from 'antd';
import { useEffect, useMemo, useRef, useState } from 'react';

import { suggestTags } from '@/api/tag';
import {
  MAX_TAGS,
  TAG_SUGGEST_DEBOUNCE_MS,
  TAG_SUGGEST_LIMIT,
} from '@/constants/tag';
import { PALETTE } from '@/constants/theme';

import { AiTagSuggest } from './AiTagSuggest';

interface TagOption {
  name: string;
  refCount: number;
}

interface TagInputProps {
  value?: string[];
  onChange?: (tags: string[]) => void;
  // 传入则在输入框下方渲染 AI 推荐；采纳候选走本组件内部 onChange（非表单 external 写，避免 rc-util isEqual 误报）
  getContext?: () => { title: string; content: string };
}

// 发文章标签输入：typeahead 补全已有标签（带引用数）+ 回车创建新标签 + AI 推荐，teal chips 展示，最多 MAX_TAGS 个。
export function TagInput({ value = [], onChange, getContext }: TagInputProps) {
  const { message } = App.useApp();
  const { token } = theme.useToken();
  const [options, setOptions] = useState<TagOption[]>([]);
  const [search, setSearch] = useState('');
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seqRef = useRef(0); // 请求序号：丢弃过期响应，避免旧请求覆盖新结果

  // 卸载时清理待触发的防抖定时器（避免 setState-after-unmount + 无谓请求）
  useEffect(
    () => () => {
      if (timer.current) {
        clearTimeout(timer.current);
      }
    },
    [],
  );

  const handleSearch = (q: string) => {
    setSearch(q);
    if (timer.current) {
      clearTimeout(timer.current);
    }
    const kw = q.trim();
    if (!kw) {
      setOptions([]);
      return;
    }
    const seq = ++seqRef.current;
    timer.current = setTimeout(() => {
      suggestTags(kw, TAG_SUGGEST_LIMIT)
        .then((res) => {
          if (seq !== seqRef.current) {
            return; // 已有更新的请求，丢弃过期响应
          }
          setOptions(
            (res.data.data ?? []).map((t) => ({
              name: t.name,
              refCount: t.refCount,
            })),
          );
        })
        .catch(() => {
          if (seq === seqRef.current) {
            setOptions([]);
          }
        });
    }, TAG_SUGGEST_DEBOUNCE_MS);
  };

  // 候选 = 后端补全项（名字 + 引用数）；输入非空且无精确同名/未选中时追加「创建新标签」合成项
  const shownOptions = useMemo<SelectProps['options']>(() => {
    const kw = search.trim();
    const opts = options.map((o) => ({
      value: o.name,
      label: (
        <div className='flex items-center justify-between'>
          <span>{o.name}</span>
          <span style={{ color: token.colorTextTertiary }}>
            {o.refCount} 篇
          </span>
        </div>
      ),
    }));
    const exists = options.some((o) => o.name === kw) || value.includes(kw);
    if (kw && !exists) {
      opts.push({
        value: kw,
        label: (
          <span style={{ color: token.colorPrimary }}>
            <PlusOutlined /> 创建标签「{kw}」
          </span>
        ),
      });
    }
    return opts;
  }, [options, search, value, token]);

  const handleChange = (tags: string[]) => {
    const cleaned = Array.from(
      new Set(tags.map((t) => t.trim()).filter(Boolean)),
    );
    if (cleaned.length > MAX_TAGS) {
      message.warning(`最多 ${MAX_TAGS} 个标签`);
      return;
    }
    setSearch('');
    setOptions([]);
    onChange?.(cleaned);
  };

  const tagRender: SelectProps['tagRender'] = ({ label, onClose }) => (
    <span
      className='mr-1 inline-flex items-center gap-1 px-2 py-0.5 text-sm'
      style={{
        background: PALETTE.tealSurface,
        color: PALETTE.primary,
        borderRadius: 12,
      }}
    >
      {label}
      <CloseOutlined
        style={{ fontSize: 10, cursor: 'pointer' }}
        onClick={onClose}
      />
    </span>
  );

  return (
    <>
      <Select
        mode='tags'
        value={value}
        onChange={handleChange}
        onSearch={handleSearch}
        options={shownOptions}
        tagRender={tagRender}
        maxCount={MAX_TAGS}
        filterOption={false}
        notFoundContent={null}
        placeholder='输入标签，回车确认...'
        style={{ width: '100%' }}
        tokenSeparators={[',', '，']}
      />
      {getContext && (
        <div className='mt-2'>
          <AiTagSuggest
            getContext={getContext}
            selected={value}
            onAdd={(name) => handleChange([...value, name])}
          />
        </div>
      )}
    </>
  );
}
