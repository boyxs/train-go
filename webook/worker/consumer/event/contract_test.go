package event_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreevt "github.com/webook/internal/events/interaction"
	workerevt "github.com/webook/worker/consumer/event"
)

// 跨服务事件契约守卫：core 生产端与 worker 消费端的 InteractionEvent 不共享代码，
// 任一端改 JSON 字段/类型或 topic 名即变红。放消费侧 external test 包，不破坏服务解耦。

// 两端 topic 名必须一致。
func TestInteractionEventContract_TopicMatches(t *testing.T) {
	assert.Equal(t, coreevt.TopicInteractionEvents, workerevt.TopicInteractionEvents,
		"core 生产 topic 与 worker 消费 topic 漂移")
}

// 两端序列化都必须等于同一份权威 JSON，单端增删字段或改 tag 即失败。
func TestInteractionEventContract_CanonicalWireFormat(t *testing.T) {
	const canonical = `{"type":"read","biz":"article","bizId":123}`

	coreJSON, err := json.Marshal(coreevt.InteractionEvent{Type: coreevt.TypeRead, Biz: "article", BizId: 123})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(coreJSON), "core 事件线格式漂移")

	workerJSON, err := json.Marshal(workerevt.InteractionEvent{Type: "read", Biz: "article", BizId: 123})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(workerJSON), "worker 事件线格式漂移")
}

// core 生产 → worker 消费 逐字段无损。
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
