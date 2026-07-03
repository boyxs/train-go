'use client';

import { App, Button, Empty, Spin } from 'antd';
import { ChevronDown } from 'lucide-react';
import { useRouter } from 'next/navigation';
import React, { useCallback, useEffect, useState } from 'react';

import * as commentApi from '@/api/comment';
import * as interactionApi from '@/api/interaction';
import * as userApi from '@/api/user';
import { BIZ } from '@/constants/biz';
import type { Comment, CommentSort } from '@/types';
import { getErrorMessage } from '@/utils/apiError';
import { tokenUtil } from '@/utils/token';

import { CommentEditor } from './CommentEditor';
import { CommentItem } from './CommentItem';

const PAGE_SIZE = 10;
const REPLIES_LIMIT = 50;
const REPLY_INDENT = 46; // 头像(36) + gap(10)，回复线程相对一级评论的左缩进
const SORTS: { key: CommentSort; label: string }[] = [
  { key: 'hot', label: '最热' },
  { key: 'new', label: '最新' },
];

interface CommentSectionProps {
  articleId: number;
}

// CommentSection 文章评论区：一级评论分页（hot/new）+ 楼内回复懒加载 + 发表/回复/赞/删除
export function CommentSection({ articleId }: CommentSectionProps) {
  const { message } = App.useApp();
  const router = useRouter();

  const [sort, setSort] = useState<CommentSort>('hot');
  const [comments, setComments] = useState<Comment[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [loggedIn, setLoggedIn] = useState(false);
  const [currentUid, setCurrentUid] = useState<number | null>(null);
  const [repliesMap, setRepliesMap] = useState<Record<number, Comment[]>>({});
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const [replyingTo, setReplyingTo] = useState<number | null>(null);
  // hasMore 基于「最近一页一级评论是否拉满」，不能拿 total（含回复的全部评论数）比一级数
  const [hasMore, setHasMore] = useState(false);

  // 当前登录态 + uid（用于「我」标记与删除权限）
  useEffect(() => {
    const ok = tokenUtil.hasToken();
    setLoggedIn(ok);
    if (ok) {
      userApi
        .findProfile()
        .then((r) => setCurrentUid(r.data.id))
        .catch(() => {});
    }
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await commentApi.listComments({
        articleId,
        sort,
        offset: 0,
        limit: PAGE_SIZE,
      });
      const list = r.data.data.list ?? [];
      setComments(list);
      setTotal(r.data.data.total ?? 0);
      // 一级评论拉满一页才可能有更多；hot 为 P0 首屏 top-N，不分页
      setHasMore(sort === 'new' && list.length >= PAGE_SIZE);
    } catch (e) {
      message.error(getErrorMessage(e, '评论加载失败'));
    } finally {
      setLoading(false);
    }
  }, [articleId, sort, message]);

  useEffect(() => {
    load();
  }, [load]);

  const changeSort = (next: CommentSort) => {
    if (next === sort) {
      return;
    }
    setRepliesMap({});
    setExpanded({});
    setReplyingTo(null);
    setSort(next);
  };

  const loadMore = async () => {
    setLoading(true);
    try {
      const r = await commentApi.listComments({
        articleId,
        sort,
        offset: comments.length,
        limit: PAGE_SIZE,
      });
      const list = r.data.data.list ?? [];
      setComments((prev) => [...prev, ...list]);
      setHasMore(list.length >= PAGE_SIZE);
    } catch (e) {
      message.error(getErrorMessage(e, '加载失败'));
    } finally {
      setLoading(false);
    }
  };

  const loadReplies = async (rootId: number) => {
    try {
      const r = await commentApi.getReplies({
        rootId,
        offset: 0,
        limit: REPLIES_LIMIT,
      });
      const list = r.data.data.list ?? [];
      setRepliesMap((prev) => ({ ...prev, [rootId]: list }));
      setExpanded((prev) => ({ ...prev, [rootId]: true }));
      // 同步楼主「展开 N 条回复」计数为实际加载数，防 reply_cnt 漂移（如删唯一回复后仍显 1 条）
      setComments((prev) =>
        prev.map((x) =>
          x.id === rootId ? { ...x, replyCnt: list.length } : x,
        ),
      );
    } catch (e) {
      message.error(getErrorMessage(e, '加载回复失败'));
    }
  };

  const submitTop = async (content: string): Promise<boolean> => {
    try {
      const r = await commentApi.createComment({ articleId, content });
      setComments((prev) => [r.data.data.comment, ...prev]);
      setTotal((t) => t + 1);
      message.success('评论成功');
      return true;
    } catch (e) {
      message.error(getErrorMessage(e, '请先登录后再评论'));
      return false;
    }
  };

  const submitReply = async (
    rootId: number,
    pid: number,
    content: string,
  ): Promise<boolean> => {
    try {
      await commentApi.createComment({ articleId, content, pid });
      setComments((prev) =>
        prev.map((x) =>
          x.id === rootId ? { ...x, replyCnt: x.replyCnt + 1 } : x,
        ),
      );
      setReplyingTo(null);
      message.success('回复成功');
      // 重拉整楼：保证原有回复 + 新回复都在（直接 append 会隐藏未加载的旧回复）
      await loadReplies(rootId);
      return true;
    } catch (e) {
      message.error(getErrorMessage(e, '回复失败'));
      return false;
    }
  };

  // applyLike 同时更新一级评论与楼内回复（点赞目标可能在任一处）
  const applyLike = (id: number, liked: boolean) => {
    const upd = (c: Comment): Comment =>
      c.id === id
        ? { ...c, liked, likeCnt: Math.max(0, c.likeCnt + (liked ? 1 : -1)) }
        : c;
    setComments((prev) => prev.map(upd));
    setRepliesMap((prev) => {
      const next: Record<number, Comment[]> = {};
      for (const k of Object.keys(prev)) {
        next[Number(k)] = prev[Number(k)].map(upd);
      }
      return next;
    });
  };

  const toggleLike = async (c: Comment) => {
    const newLiked = !c.liked;
    applyLike(c.id, newLiked); // 乐观更新
    try {
      await interactionApi.like({
        biz: BIZ.COMMENT,
        bizId: c.id,
        liked: newLiked,
      });
    } catch (e) {
      applyLike(c.id, !newLiked); // 回滚
      message.error(getErrorMessage(e, '请先登录后再操作'));
    }
  };

  const remove = async (c: Comment) => {
    try {
      await commentApi.deleteComment(c.id);
      message.success('已删除');
      // 删一级评论整楼级联：清掉该楼回复缓存与展开态再重拉
      if (c.rootId === 0) {
        setRepliesMap((prev) => {
          const next = { ...prev };
          delete next[c.id];
          return next;
        });
        setExpanded((prev) => {
          const next = { ...prev };
          delete next[c.id];
          return next;
        });
        await load();
      } else {
        await loadReplies(c.rootId);
      }
    } catch (e) {
      message.error(getErrorMessage(e, '删除失败'));
    }
  };

  const toggleReply = (c: Comment) =>
    setReplyingTo((prev) => (prev === c.id ? null : c.id));

  return (
    <div
      className='bg-white rounded-xl flex flex-col gap-4 mt-4'
      style={{ padding: '20px 24px' }}
    >
      {/* 标题 + 排序 */}
      <div className='flex items-center justify-between'>
        <div className='flex items-end gap-1.5'>
          <span style={{ fontSize: 16, fontWeight: 700, color: '#1A1A1A' }}>
            评论
          </span>
          <span style={{ fontSize: 13, color: '#9CA3AF' }}>{total} 条</span>
        </div>
        <div
          className='flex items-center'
          style={{ background: '#F5F5F5', borderRadius: 8, padding: 3, gap: 4 }}
        >
          {SORTS.map((s) => {
            const active = s.key === sort;
            return (
              <button
                key={s.key}
                type='button'
                onClick={() => changeSort(s.key)}
                className='border-none cursor-pointer'
                style={{
                  background: active ? '#FFFFFF' : 'transparent',
                  color: active ? '#0D9488' : '#6B7280',
                  fontSize: 13,
                  fontWeight: active ? 600 : 400,
                  borderRadius: 6,
                  padding: '5px 12px',
                }}
              >
                {s.label}
              </button>
            );
          })}
        </div>
      </div>

      {/* 顶部输入框 / 登录提示 */}
      {loggedIn ? (
        <CommentEditor onSubmit={submitTop} />
      ) : (
        <button
          type='button'
          onClick={() => router.push('/login')}
          className='rounded-lg cursor-pointer text-left'
          style={{
            border: '1px solid #E5E7EB',
            padding: '12px 14px',
            color: '#9CA3AF',
            fontSize: 14,
            background: 'transparent',
          }}
        >
          登录后参与评论…
        </button>
      )}

      {/* 列表 */}
      {loading && comments.length === 0 ? (
        <div className='flex justify-center py-8'>
          <Spin />
        </div>
      ) : comments.length === 0 ? (
        <Empty
          description='还没有评论，来抢沙发'
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      ) : (
        <div className='flex flex-col'>
          {comments.map((c, idx) => {
            const replies = repliesMap[c.id] ?? [];
            const isExpanded = expanded[c.id];
            return (
              <div
                key={c.id}
                style={{
                  padding: '16px 0',
                  borderTop: idx === 0 ? undefined : '1px solid #F3F4F6',
                }}
              >
                <CommentItem
                  comment={c}
                  currentUid={currentUid}
                  onLike={toggleLike}
                  onReply={toggleReply}
                  onDelete={remove}
                />

                {replyingTo === c.id && (
                  <div style={{ marginLeft: REPLY_INDENT, marginTop: 10 }}>
                    <CommentEditor
                      autoFocus
                      replyTo={c.user.name}
                      submitText='回复'
                      onSubmit={(txt) => submitReply(c.id, c.id, txt)}
                      onCancel={() => setReplyingTo(null)}
                    />
                  </div>
                )}

                {/* 楼内回复（整楼扁平展示，统一一层缩进） */}
                {isExpanded && replies.length > 0 && (
                  <div
                    className='flex flex-col gap-3.5'
                    style={{
                      marginLeft: REPLY_INDENT,
                      marginTop: 12,
                      paddingLeft: 14,
                      borderLeft: '2px solid #E5E7EB',
                    }}
                  >
                    {replies.map((rep) => (
                      <div key={rep.id} className='flex flex-col gap-2.5'>
                        <CommentItem
                          comment={rep}
                          isReply
                          currentUid={currentUid}
                          onLike={toggleLike}
                          onReply={toggleReply}
                          onDelete={remove}
                        />
                        {replyingTo === rep.id && (
                          <CommentEditor
                            autoFocus
                            replyTo={rep.user.name}
                            submitText='回复'
                            onSubmit={(txt) => submitReply(c.id, rep.id, txt)}
                            onCancel={() => setReplyingTo(null)}
                          />
                        )}
                      </div>
                    ))}
                  </div>
                )}

                {/* 展开回复 */}
                {!isExpanded && c.replyCnt > 0 && (
                  <button
                    type='button'
                    onClick={() => loadReplies(c.id)}
                    className='flex items-center gap-1.5 bg-transparent border-none cursor-pointer p-0'
                    style={{ marginLeft: REPLY_INDENT, marginTop: 10 }}
                  >
                    <ChevronDown size={15} color='#0D9488' />
                    <span
                      style={{
                        fontSize: 13,
                        fontWeight: 600,
                        color: '#0D9488',
                      }}
                    >
                      展开 {c.replyCnt} 条回复
                    </span>
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* 加载更多 */}
      {hasMore && (
        <div className='flex justify-center'>
          <Button onClick={loadMore} loading={loading}>
            加载更多评论
          </Button>
        </div>
      )}
    </div>
  );
}
