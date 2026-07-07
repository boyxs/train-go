package relation

const TopicRelationEvents = "relation_events"

// 事件类型取值（RelationEvent.Type）。
const (
	TypeFollow   = "follow"
	TypeUnfollow = "unfollow"
	TypeBlock    = "block"
	TypeUnblock  = "unblock"
)

// RelationEvent Kafka 关系事件。core 在关系写成功且 changed=true 时生产；relation gRPC 服务纯同步不产事件。
// 消费方（P1 worker「被关注」通知 / 未来 feed 扩散）各自定义同构结构（topic+JSON 契约，两端不共享代码）。
// follow/unfollow：follower→followee；block/unblock：follower=拉黑发起者、followee=被拉黑者。
type RelationEvent struct {
	Type       string `json:"type"`       // 见 TypeFollow 等常量
	FollowerId int64  `json:"followerId"` // 关注者 / 拉黑发起者
	FolloweeId int64  `json:"followeeId"` // 被关注者 / 被拉黑者
	Ts         int64  `json:"ts"`         // 事件时间（Unix 毫秒）
}
