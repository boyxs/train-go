package domain

// RelationStats 用户关系聚合计数。
type RelationStats struct {
	Uid         int64 `json:"uid"`
	FolloweeCnt int64 `json:"followeeCnt"` // 关注数：该用户关注了多少人
	FollowerCnt int64 `json:"followerCnt"` // 粉丝数：多少人关注该用户
}

// FollowEdge 一条关注边（列表项）；列表方按需取 follower / followee 侧。Id 作游标。
type FollowEdge struct {
	Id         int64 `json:"id"`
	FollowerId int64 `json:"followerId"`
	FolloweeId int64 `json:"followeeId"`
	CreatedAt  int64 `json:"createdAt"`
}

// BlockEdge 一条拉黑记录（黑名单项）。Id 作游标。
type BlockEdge struct {
	Id         int64 `json:"id"`
	Uid        int64 `json:"uid"`
	BlockedUid int64 `json:"blockedUid"`
	CreatedAt  int64 `json:"createdAt"`
}

// RelationState viewer 对某个 target 的关系态（供关注按钮态 / 列表标记）。
type RelationState struct {
	IsFollowing  bool `json:"isFollowing"`  // viewer 关注了 target
	IsFollowedBy bool `json:"isFollowedBy"` // target 关注了 viewer
	IsBlocked    bool `json:"isBlocked"`    // viewer 拉黑了 target
	IsBlockedBy  bool `json:"isBlockedBy"`  // target 拉黑了 viewer
}

// IsMutual 互相关注 = 双向关注（拉黑时双向关注已被解除，故拉黑与互关互斥）。
func (rs RelationState) IsMutual() bool { return rs.IsFollowing && rs.IsFollowedBy }
