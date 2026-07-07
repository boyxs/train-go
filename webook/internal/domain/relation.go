package domain

// RelationUser 关系列表项（关注 / 粉丝 / 黑名单通用）——
// core 聚合 relation gRPC（边 + 计数 + 关系态）与 user 昵称后的领域视图，接入层再映射为各自 VO。
type RelationUser struct {
	Id             int64
	Name           string
	Bio            string
	IsMutual       bool  // 关注列表：与列表主人互关
	IsFollowedBack bool  // 粉丝列表：列表主人已回关
	FolloweeCnt    int64 // 该用户关注数
	FollowerCnt    int64 // 该用户粉丝数
	CreatedAt      int64 // 粉丝列表 = 关注时间；黑名单 = 拉黑时间
}

// RelationStat 某用户计数 + viewer 对其关系态。
type RelationStat struct {
	FolloweeCnt int64
	FollowerCnt int64
	IsFollowing bool // viewer 关注了对方
	IsMutual    bool // 互相关注
	IsBlocked   bool // viewer 拉黑了对方
	IsBlockedBy bool // 对方拉黑了 viewer
}
