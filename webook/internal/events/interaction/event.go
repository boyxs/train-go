package interaction

const TopicInteractionEvents = "interaction_events"

// InteractionEvent Kafka 互动事件
type InteractionEvent struct {
	Type  string `json:"type"`  // "read"
	Biz   string `json:"biz"`   // 业务类型，如 "article"
	BizId int64  `json:"bizId"` // 业务 ID
}
