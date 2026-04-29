package integration

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/webook/chat/domain"
	"github.com/webook/chat/repository"
	"github.com/webook/chat/repository/cache"
	"github.com/webook/chat/repository/dao"
	"github.com/webook/pkg/logger"
)

// MessageRepoSuite 真实 mysql + redis；覆盖消息 CRUD、Cache-Aside、Lite 字段裁剪、反馈更新等。
type MessageRepoSuite struct {
	suite.Suite
	db   *gorm.DB
	cmd  redis.Cmdable
	repo repository.MessageRepository
}

func TestMessageRepo(t *testing.T) {
	suite.Run(t, &MessageRepoSuite{})
}

func (s *MessageRepoSuite) SetupSuite() {
	db, err := gorm.Open(mysql.Open(viper.GetString("mysql.dsn")))
	require.NoError(s.T(), err)
	require.NoError(s.T(), dao.InitTable(db))

	cmd := redis.NewClient(&redis.Options{
		Addr:     viper.GetString("redis.addr"),
		Password: viper.GetString("redis.password"),
	})
	s.db = db
	s.cmd = cmd
	s.repo = repository.NewCacheMessageRepository(
		dao.NewGormMessageDAO(db),
		cache.NewRedisMessageCache(cmd),
		logger.NewNopLogger(),
	)
}

func (s *MessageRepoSuite) TearDownTest() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE conversation").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE message").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

func (s *MessageRepoSuite) TestInsert_AssignsId_FillsTimestamp() {
	t := s.T()
	msg, err := s.repo.Insert(context.Background(), domain.Message{
		ConversationId: 1, Role: "user", Content: "hello",
	})
	require.NoError(t, err)
	assert.Greater(t, msg.Id, int64(0))
	assert.Greater(t, msg.CreatedAt, int64(0))
	assert.Equal(t, "hello", msg.Content)
}

func (s *MessageRepoSuite) TestListRecent_ReturnsAscOrder_AndCacheFillback() {
	t := s.T()
	ctx := context.Background()
	convId := int64(10)

	for i := 0; i < 5; i++ {
		_, err := s.repo.Insert(ctx, domain.Message{
			ConversationId: convId, Role: "user", Content: string(rune('a' + i)),
		})
		require.NoError(t, err)
	}

	// 取最近 3 条；DAO 内部 DESC 取后再反转 ASC，最近 3 条按时间正序
	got, err := s.repo.ListRecent(ctx, convId, 3)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "c", got[0].Content)
	assert.Equal(t, "d", got[1].Content)
	assert.Equal(t, "e", got[2].Content)
}

func (s *MessageRepoSuite) TestListRecent_CacheHit_WhenSizeSufficient() {
	t := s.T()
	ctx := context.Background()
	convId := int64(20)

	for i := 0; i < 4; i++ {
		_, err := s.repo.Insert(ctx, domain.Message{
			ConversationId: convId, Role: "user", Content: string(rune('a' + i)),
		})
		require.NoError(t, err)
	}

	// 第一次 list 触发缓存回填（缓存 4 条）
	first, err := s.repo.ListRecent(ctx, convId, 4)
	require.NoError(t, err)
	require.Len(t, first, 4)

	// 改 DB 一条 content，但不清缓存：再次 list（缓存 ≥ limit）应返回旧数据
	require.NoError(t, s.db.Exec("UPDATE message SET content = 'TAINTED' WHERE conversation_id = ?", convId).Error)

	cached, err := s.repo.ListRecent(ctx, convId, 4)
	require.NoError(t, err)
	require.Len(t, cached, 4)
	for _, m := range cached {
		assert.NotEqual(t, "TAINTED", m.Content, "应返回缓存中的旧数据，证明走了缓存")
	}
}

func (s *MessageRepoSuite) TestListRecent_CacheMissWhenSizeInsufficient() {
	t := s.T()
	ctx := context.Background()
	convId := int64(30)

	for i := 0; i < 2; i++ {
		_, err := s.repo.Insert(ctx, domain.Message{
			ConversationId: convId, Role: "user", Content: string(rune('a' + i)),
		})
		require.NoError(t, err)
	}

	// 缓存里现在最多 2 条；要 5 条 → 缓存不够 → 必须回源 DB
	got, err := s.repo.ListRecent(ctx, convId, 5)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func (s *MessageRepoSuite) TestListBefore_Pagination() {
	t := s.T()
	ctx := context.Background()
	convId := int64(40)

	var ids []int64
	for i := 0; i < 5; i++ {
		m, err := s.repo.Insert(ctx, domain.Message{
			ConversationId: convId, Role: "user", Content: "msg",
		})
		require.NoError(t, err)
		ids = append(ids, m.Id)
	}

	// 取 ids[3] 之前的 2 条 = ids[1], ids[2]（按 ASC 返回）
	got, err := s.repo.ListBefore(ctx, convId, ids[3], 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, ids[1], got[0].Id)
	assert.Equal(t, ids[2], got[1].Id)
}

func (s *MessageRepoSuite) TestListRecentLite_ExcludesToolCalls() {
	t := s.T()
	ctx := context.Background()
	convId := int64(50)

	_, err := s.repo.Insert(ctx, domain.Message{
		ConversationId: convId, Role: "assistant",
		Content: "with-tools", ToolCalls: `[{"id":"x"}]`,
	})
	require.NoError(t, err)

	// Lite 版本走 Select 排除 tool_calls，回来的 ToolCalls 应为空字符串
	got, err := s.repo.ListRecentLite(ctx, convId, 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].ToolCalls)
	assert.Equal(t, "with-tools", got[0].Content)
}

func (s *MessageRepoSuite) TestUpdateContent_PersistsToDB() {
	t := s.T()
	ctx := context.Background()
	convId := int64(60)

	msg, err := s.repo.Insert(ctx, domain.Message{
		ConversationId: convId, Role: "assistant", Content: "",
	})
	require.NoError(t, err)

	require.NoError(t, s.repo.UpdateContent(ctx, convId, msg.Id, "final reply", `[{"id":"y"}]`))

	// 直接读 DAO 验落库（避开缓存影响）
	var raw dao.Message
	require.NoError(t, s.db.Where("id = ?", msg.Id).First(&raw).Error)
	assert.Equal(t, "final reply", raw.Content)
	require.NotNil(t, raw.ToolCalls)
	// MySQL JSON 列存进去会规范化（冒号后加空格），按语义比较
	assert.JSONEq(t, `[{"id":"y"}]`, *raw.ToolCalls)
}

func (s *MessageRepoSuite) TestUpdateFeedback_UpdatesAndInvalidatesCache() {
	t := s.T()
	ctx := context.Background()
	convId := int64(70)

	msg, err := s.repo.Insert(ctx, domain.Message{
		ConversationId: convId, Role: "assistant", Content: "x",
	})
	require.NoError(t, err)

	// 让缓存先有数据
	_, err = s.repo.ListRecent(ctx, convId, 10)
	require.NoError(t, err)

	require.NoError(t, s.repo.UpdateFeedback(ctx, convId, msg.Id, 1))

	// UpdateFeedback 内部清缓存 → 再 ListRecent 走 DAO 拿最新 feedback
	got, err := s.repo.ListRecent(ctx, convId, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int8(1), got[0].Feedback)
}

func (s *MessageRepoSuite) TestInsert_InvalidatesListCache() {
	t := s.T()
	ctx := context.Background()
	convId := int64(80)

	// 先建一条预热缓存
	_, err := s.repo.Insert(ctx, domain.Message{
		ConversationId: convId, Role: "user", Content: "first",
	})
	require.NoError(t, err)
	_, err = s.repo.ListRecent(ctx, convId, 10)
	require.NoError(t, err)

	// 再 Insert 一条
	_, err = s.repo.Insert(ctx, domain.Message{
		ConversationId: convId, Role: "user", Content: "second",
	})
	require.NoError(t, err)

	// 缓存被 Insert 清了；ListRecent 走 DAO 应返回 2 条
	got, err := s.repo.ListRecent(ctx, convId, 10)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
