package event_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corearticleevt "github.com/boyxs/train-go/webook/internal/events/article"
	coreevt "github.com/boyxs/train-go/webook/internal/events/interaction"
	corerelationevt "github.com/boyxs/train-go/webook/internal/events/relation"
	workerevt "github.com/boyxs/train-go/webook/worker/consumer/event"
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

// ── article_events 契约（feed 写扩散/移除）──────────────────────────────

func TestArticleEventContract_TopicMatches(t *testing.T) {
	assert.Equal(t, corearticleevt.TopicArticleEvents, workerevt.TopicArticleEvents,
		"core 生产 topic 与 worker 消费 topic 漂移")
}

func TestArticleEventContract_CanonicalWireFormat(t *testing.T) {
	const canonical = `{"type":"published","articleId":1001,"authorId":7,"ts":1770000000000}`

	coreJSON, err := json.Marshal(corearticleevt.ArticleEvent{
		Type: corearticleevt.TypePublished, ArticleId: 1001, AuthorId: 7, Ts: 1770000000000,
	})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(coreJSON), "core article 事件线格式漂移")

	workerJSON, err := json.Marshal(workerevt.ArticleEvent{
		Type: workerevt.ArticleTypePublished, ArticleId: 1001, AuthorId: 7, Ts: 1770000000000,
	})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(workerJSON), "worker article 事件线格式漂移")
}

func TestArticleEventContract_RoundTripLossless(t *testing.T) {
	produced := corearticleevt.ArticleEvent{Type: corearticleevt.TypeWithdrawn, ArticleId: 42, AuthorId: 9, Ts: 123}
	data, err := json.Marshal(produced)
	require.NoError(t, err)
	var consumed workerevt.ArticleEvent
	require.NoError(t, json.Unmarshal(data, &consumed), "worker 无法反序列化 core article 事件——契约漂移")
	assert.Equal(t, produced.Type, consumed.Type)
	assert.Equal(t, produced.ArticleId, consumed.ArticleId)
	assert.Equal(t, produced.AuthorId, consumed.AuthorId)
	assert.Equal(t, produced.Ts, consumed.Ts)
}

// ── relation_events 契约（feed 失效重建）──────────────────────────────

func TestRelationEventContract_TopicMatches(t *testing.T) {
	assert.Equal(t, corerelationevt.TopicRelationEvents, workerevt.TopicRelationEvents,
		"core 生产 topic 与 worker 消费 topic 漂移")
}

func TestRelationEventContract_CanonicalWireFormat(t *testing.T) {
	const canonical = `{"type":"block","followerId":7,"followeeId":8,"ts":1770000000000}`

	coreJSON, err := json.Marshal(corerelationevt.RelationEvent{
		Type: corerelationevt.TypeBlock, FollowerId: 7, FolloweeId: 8, Ts: 1770000000000,
	})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(coreJSON), "core relation 事件线格式漂移")

	workerJSON, err := json.Marshal(workerevt.RelationEvent{
		Type: workerevt.RelationTypeBlock, FollowerId: 7, FolloweeId: 8, Ts: 1770000000000,
	})
	require.NoError(t, err)
	assert.JSONEq(t, canonical, string(workerJSON), "worker relation 事件线格式漂移")
}

func TestRelationEventContract_RoundTripLossless(t *testing.T) {
	produced := corerelationevt.RelationEvent{Type: corerelationevt.TypeFollow, FollowerId: 3, FolloweeId: 5, Ts: 99}
	data, err := json.Marshal(produced)
	require.NoError(t, err)
	var consumed workerevt.RelationEvent
	require.NoError(t, json.Unmarshal(data, &consumed), "worker 无法反序列化 core relation 事件——契约漂移")
	assert.Equal(t, produced.Type, consumed.Type)
	assert.Equal(t, produced.FollowerId, consumed.FollowerId)
	assert.Equal(t, produced.FolloweeId, consumed.FolloweeId)
}
