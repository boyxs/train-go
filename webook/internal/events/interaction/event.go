package interaction

const TopicInteractionEvents = "interaction_events"

// 事件类型取值（InteractionEvent.Type）。当前只有 read，新增类型在此扩充。
const TypeRead = "read"

// InteractionEvent Kafka 互动事件
type InteractionEvent struct {
	Type  string `json:"type"`  // 事件类型，见 TypeRead 等常量
	Biz   string `json:"biz"`   // 业务类型，如 "article"
	BizId int64  `json:"bizId"` // 业务 ID
}
