// 对应后端 core web relation VO（webook/internal/web/relation.go）
// relation 是独立 gRPC 服务，core 聚合 user 昵称/简介后以下列形态回前端。

// POST /relation/stat — 某用户计数 + viewer 对其关系态
export interface RelationStat {
  followeeCnt: number;
  followerCnt: number;
  isFollowing: boolean; // viewer 已关注对方
  isMutual: boolean; // 互相关注
  isBlocked: boolean; // viewer 拉黑了对方
  isBlockedBy: boolean; // 对方拉黑了 viewer（前端置灰关注按钮）
}

// 用户简介（列表项通用；头像前端按 name 首字母渲染，user 表无头像字段）
export interface UserBrief {
  id: number;
  name: string;
  bio: string;
}

// 关注列表项：该关注对象是否也关注了列表主人
export interface FolloweeItem extends UserBrief {
  isMutual: boolean;
  followeeCnt: number; // 该用户关注数
  followerCnt: number; // 该用户粉丝数
}

// 粉丝列表项：列表主人是否已回关该粉丝
export interface FollowerItem extends UserBrief {
  isFollowedBack: boolean;
  followeeCnt: number; // 该用户关注数
  followerCnt: number; // 该用户粉丝数
  createdAt: number; // 该粉丝关注列表主人的时间（关注了你 · X）
}

// 黑名单项
export interface BlockItem extends UserBrief {
  blockedAt: number;
}

// 关系列表统一游标响应（对齐 core web：{list, nextCursor}，非 offset PageResult）
export interface CursorList<T> {
  list: T[];
  nextCursor: number;
}

// POST /relation/followees | /relation/followers
export interface RelationListReq {
  userId: number;
  cursor?: number;
  limit?: number;
}

// POST /relation/blocklist
export interface BlocklistReq {
  cursor?: number;
  limit?: number;
}
