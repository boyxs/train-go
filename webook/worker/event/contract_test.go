package event_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreevt "github.com/webook/internal/events/interaction"
	workerevt "github.com/webook/worker/event"
)

// 跨服务事件契约守卫。
//
// core 生产侧（internal/events/interaction）与 worker 消费侧（worker/event）的 InteractionEvent
// 刻意不共享代码（"broker 即边界，两端各自定义"）——代价是没有编译期约束、纯靠纪律防漂移。
// 本测试就是补回的那张安全网：任一端改了 JSON 字段名/类型 或 topic 名，这里立即变红，逼两端同步改。
//
// 放在 worker（消费侧）的 external test 包（package event_test）：worker 的生产代码不依赖 core，
// 只有本契约测试在编译期同时拉入两端结构做比对，不破坏服务解耦。

// topic 名两端必须一致，否则 worker 根本订不到 core 投递的分区。
func TestInteractionEventContract_TopicMatches(t *testing.T) {
	assert.Equal(t, coreevt.TopicInteractionEvents, workerevt.TopicInteractionEvents,
		"core 生产 topic 与 worker 消费 topic 漂移")
}

// canonical 线格式：固定一份权威 JSON，两端序列化结果都必须等于它。
// 任一端增/删字段或改 json tag → JSONEq byte 级不等立即失败（这是检测跨端漂移的核心断言：
// 两端都对齐同一 canonical，单端漂移就只有该端红）。
func TestInteractionEventContract_CanonicalWireFormat(t *testing.T) {
	const canonical = `{"type":"read","biz":"article","bizId":123}`

	coreJSON, err := json.Marshal(coreevt.InteractionEvent{Type: coreevt.TypeRead, Biz: "article", BizId: 123})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(coreJSON), "core 事件线格式漂移")

	workerJSON, err := json.Marshal(workerevt.InteractionEvent{Type: "read", Biz: "article", BizId: 123})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(workerJSON), "worker 事件线格式漂移")
}

// core 生产 → worker 消费 逐字段无损（反序列化方向的兜底）。
func TestInteractionEventContract_RoundTripLossless(t *testing.T) {
	produced := coreevt.InteractionEvent{Type: coreevt.TypeRead, Biz: "article", BizId: 12345}
	data, err := json.Marshal(produced)
	require.NoError(t, err)

	var consumed workerevt.InteractionEvent
	require.NoError(t, json.Unmarshal(data, &consumed),
		"worker 无法反序列化 core 生产的事件——字段契约已漂移")
	assert.Equal(t, produced.Type, consumed.Type)
	assert.Equal(t, produced.Biz, consumed.Biz)
	assert.Equal(t, produced.BizId, consumed.BizId)
}
