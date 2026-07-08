package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/chat/domain"
	"github.com/boyxs/train-go/webook/chat/errs"
	"github.com/boyxs/train-go/webook/chat/repository"
	"github.com/boyxs/train-go/webook/chat/repository/cache"
	"github.com/boyxs/train-go/webook/chat/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ConversationRepoSuite 直接接真实 mysql + redis，跳过 wire（避免 LLM/gRPC 全栈起来）。
// 覆盖 Cache-Aside、软删除、列表缓存失效等核心路径。
type ConversationRepoSuite struct {
	suite.Suite
	db   *gorm.DB
	cmd  redis.Cmdable
	repo repository.ConversationRepository
}

func TestConversationRepo(t *testing.T) {
	suite.Run(t, &ConversationRepoSuite{})
}

func (s *ConversationRepoSuite) SetupSuite() {
	db, err := gorm.Open(mysql.Open(viper.GetString("data.mysql.dsn")))
	require.NoError(s.T(), err)
	require.NoError(s.T(), dao.InitTable(db))

	cmd := redis.NewClient(&redis.Options{
		Addr:     viper.GetString("data.redis.addr"),
		Password: viper.GetString("data.redis.password"),
	})
	s.db = db
	s.cmd = cmd
	s.repo = repository.NewCacheConversationRepository(
		dao.NewGormConversationDAO(db),
		cache.NewRedisConversationCache(cmd),
		logger.NewNopLogger(),
	)
}

// TearDownTest 每个用例后清表 + 清 Redis；TRUNCATE 同时清掉软删除残留行
func (s *ConversationRepoSuite) TearDownTest() {
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE conversation").Error)
	require.NoError(s.T(), s.db.Exec("TRUNCATE TABLE message").Error)
	require.NoError(s.T(), s.cmd.FlushDB(context.Background()).Err())
}

func (s *ConversationRepoSuite) TestCreate_AssignsId_FillsTimestamps() {
	t := s.T()
	ctx := context.Background()

	conv, err := s.repo.Create(ctx, domain.Conversation{UserId: 100, Title: "新对话"})
	require.NoError(t, err)
	assert.Greater(t, conv.Id, int64(0))
	assert.Greater(t, conv.CreatedAt, int64(0))
	assert.Greater(t, conv.UpdatedAt, int64(0))
	assert.Equal(t, "新对话", conv.Title)
}

func (s *ConversationRepoSuite) TestList_CacheMiss_HitsDB_AndFillsBack() {
	t := s.T()
	ctx := context.Background()

	c1, err := s.repo.Create(ctx, domain.Conversation{UserId: 200, Title: "A"})
	require.NoError(t, err)
	// 制造 updated_at 差异确保排序稳定（autoUpdateTime:milli）
	time.Sleep(2 * time.Millisecond)
	c2, err := s.repo.Create(ctx, domain.Conversation{UserId: 200, Title: "B"})
	require.NoError(t, err)

	// Create 内部清了缓存；首次 List 必然 miss → 走 DAO → 回填
	got, err := s.repo.List(ctx, 200)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// updated_at DESC（最新创建的 c2 在前）
	assert.Equal(t, c2.Id, got[0].Id)
	assert.Equal(t, c1.Id, got[1].Id)

	// 再 List 一次走缓存（DAO 不会再被打到，但我们没法验证；仅验语义一致性）
	cached, err := s.repo.List(ctx, 200)
	require.NoError(t, err)
	assert.Equal(t, got, cached)
}

func (s *ConversationRepoSuite) TestList_OnlyOwnerVisible() {
	t := s.T()
	ctx := context.Background()

	_, err := s.repo.Create(ctx, domain.Conversation{UserId: 300, Title: "u300"})
	require.NoError(t, err)
	_, err = s.repo.Create(ctx, domain.Conversation{UserId: 301, Title: "u301"})
	require.NoError(t, err)

	got300, err := s.repo.List(ctx, 300)
	require.NoError(t, err)
	require.Len(t, got300, 1)
	assert.Equal(t, "u300", got300[0].Title)
}

func (s *ConversationRepoSuite) TestFind_NotFound() {
	t := s.T()
	_, err := s.repo.Find(context.Background(), 1, 99999)
	assert.True(t, errors.Is(err, errs.ErrRecordNotFound))
}

func (s *ConversationRepoSuite) TestFind_NotOwner_ReturnsNotFound() {
	t := s.T()
	ctx := context.Background()
	conv, err := s.repo.Create(ctx, domain.Conversation{UserId: 400, Title: "owned-by-400"})
	require.NoError(t, err)

	// uid 不匹配：Find WHERE id AND user_id 一起，查不到等同 NotFound
	_, err = s.repo.Find(ctx, 401, conv.Id)
	assert.True(t, errors.Is(err, errs.ErrRecordNotFound))
}

func (s *ConversationRepoSuite) TestUpdateTitle_InvalidatesCache() {
	t := s.T()
	ctx := context.Background()
	conv, err := s.repo.Create(ctx, domain.Conversation{UserId: 500, Title: "old"})
	require.NoError(t, err)

	// 先 List 一次让缓存有内容
	_, err = s.repo.List(ctx, 500)
	require.NoError(t, err)

	require.NoError(t, s.repo.UpdateTitle(ctx, 500, conv.Id, "new"))

	// 缓存被清后 List 走 DAO 拿到新 title
	got, err := s.repo.List(ctx, 500)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0].Title)
}

func (s *ConversationRepoSuite) TestDelete_SoftDelete_AndCascade() {
	t := s.T()
	ctx := context.Background()
	conv, err := s.repo.Create(ctx, domain.Conversation{UserId: 600, Title: "to-delete"})
	require.NoError(t, err)

	// 先存几条 message，验证级联软删
	require.NoError(t, s.db.Create(&dao.Message{
		ConversationId: conv.Id, Role: "user", Content: "hi",
	}).Error)

	require.NoError(t, s.repo.Delete(ctx, 600, conv.Id))

	// List 不再返回（GORM softDelete 自动过滤 deleted_at != 0）
	got, err := s.repo.List(ctx, 600)
	require.NoError(t, err)
	assert.Empty(t, got)

	// Find 也找不到
	_, err = s.repo.Find(ctx, 600, conv.Id)
	assert.True(t, errors.Is(err, errs.ErrRecordNotFound))

	// 但物理记录还在，deleted_at 非 0（用 Unscoped 查证）
	var raw dao.Conversation
	require.NoError(t, s.db.Unscoped().Where("id = ?", conv.Id).First(&raw).Error)
	assert.NotZero(t, raw.DeletedAt)

	// message 级联软删：Unscoped 能查到，正常 Find 拿不到
	var msgRaw []dao.Message
	require.NoError(t, s.db.Unscoped().Where("conversation_id = ?", conv.Id).Find(&msgRaw).Error)
	require.Len(t, msgRaw, 1)
	assert.NotZero(t, msgRaw[0].DeletedAt)
}

func (s *ConversationRepoSuite) TestDelete_NonExistent_ReturnsNotFound() {
	err := s.repo.Delete(context.Background(), 700, 99999)
	assert.True(s.T(), errors.Is(err, errs.ErrRecordNotFound))
}
