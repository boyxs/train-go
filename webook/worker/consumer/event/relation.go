package event

// RelationEvent 与 core 生产端约定的关系事件（跨服务契约，两端各自定义、不共享代码）。
// feed 失效重建的源头：follow/unfollow 失效 follower；block 失效双方；unblock 跳过。
type RelationEvent struct {
	Type       string `json:"type"`       // follow / unfollow / block / unblock
	FollowerId int64  `json:"followerId"` // 关注者 / 拉黑发起者
	FolloweeId int64  `json:"followeeId"` // 被关注者 / 被拉黑者
	Ts         int64  `json:"ts"`         // 事件时间（Unix 毫秒）
}

// TopicRelationEvents 必须与 core 生产端一致。
const TopicRelationEvents = "relation_events"

// 事件类型取值（RelationEvent.Type），与 core 一致。
const (
	RelationTypeFollow   = "follow"
	RelationTypeUnfollow = "unfollow"
	RelationTypeBlock    = "block"
	RelationTypeUnblock  = "unblock"
)
