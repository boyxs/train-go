// Package event 定义 worker 消费侧的事件线格式契约（数据类型 + topic 常量），无运行时。
package event

// InteractionEvent 与 core 生产端约定的 read 事件（跨服务契约，两端各自定义）。
type InteractionEvent struct {
	Type  string `json:"type"`  // read
	Biz   string `json:"biz"`   // 业务类型，如 article
	BizId int64  `json:"bizId"` // 业务 ID
}

// TopicInteractionEvents 必须与 core 生产端一致。
const TopicInteractionEvents = "interaction_events"
