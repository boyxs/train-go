package event

// InteractionEvent 与 webook-core 生产侧约定的 read 事件。
// 跨服务契约 = topic 名 + JSON 结构（broker 即边界），两端各自定义、不共享代码。
type InteractionEvent struct {
	Type  string `json:"type"`  // read
	Biz   string `json:"biz"`   // 业务类型，如 article
	BizId int64  `json:"bizId"` // 业务 ID
}

// TopicInteractionEvents 必须与 core 生产端一致。
const TopicInteractionEvents = "interaction_events"
