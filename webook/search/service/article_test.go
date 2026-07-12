package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/search/domain"
	"github.com/boyxs/train-go/webook/search/errs"
	"github.com/boyxs/train-go/webook/search/repository"
)

// 手写桩隔离 ES / embedding：单测覆盖 service 的校验 / 分页 clamp / 降级 / 幂等编排，不依赖真实中间件（常规 CI 可跑）。

type stubEmbedder struct {
	vec []float32
	err error
}

func (e stubEmbedder) Embed(context.Context, string) ([]float32, error) { return e.vec, e.err }

type stubRepo struct {
	indexErr            error
	indexCalled         bool
	removeErr           error
	searchRes           domain.SearchResult
	searchErr           error
	searchCalled        bool
	gotOffset, gotLimit int
	recTags             []domain.TagCount
	recErr              error
	recCalled           bool
}

var _ repository.ArticleRepository = (*stubRepo)(nil)

func (r *stubRepo) Index(context.Context, domain.Article, []float32) error {
	r.indexCalled = true
	return r.indexErr
}
func (r *stubRepo) Remove(context.Context, int64) error { return r.removeErr }
func (r *stubRepo) Search(_ context.Context, _ string, _ []float32, _ []string, offset, limit int) (domain.SearchResult, error) {
	r.searchCalled, r.gotOffset, r.gotLimit = true, offset, limit
	return r.searchRes, r.searchErr
}
func (r *stubRepo) RecommendTags(context.Context, []float32, int) ([]domain.TagCount, error) {
	r.recCalled = true
	return r.recTags, r.recErr
}

func newSvc(e Embedder, r repository.ArticleRepository) ArticleService {
	return NewInternalArticleService(r, e, logger.NewNopLogger())
}

// Search：空 query → ErrSearchQueryEmpty，不触达 repo
func TestSearch_EmptyQuery(t *testing.T) {
	repo := &stubRepo{}
	_, err := newSvc(stubEmbedder{vec: []float32{1}}, repo).Search(context.Background(), "   ", nil, 1, 10)
	assert.ErrorIs(t, err, errs.ErrSearchQueryEmpty)
	assert.False(t, repo.searchCalled)
}

// Search：超长 query → ErrSearchQueryTooLong
func TestSearch_TooLong(t *testing.T) {
	repo := &stubRepo{}
	_, err := newSvc(stubEmbedder{vec: []float32{1}}, repo).Search(context.Background(), strings.Repeat("x", maxQueryRunes+1), nil, 1, 10)
	assert.ErrorIs(t, err, errs.ErrSearchQueryTooLong)
	assert.False(t, repo.searchCalled)
}

// Search：size 超上限截断到 maxSearchSize；page/size 归一后正确算 offset
func TestSearch_SizeClampAndOffset(t *testing.T) {
	repo := &stubRepo{}
	_, err := newSvc(stubEmbedder{vec: []float32{1}}, repo).Search(context.Background(), "go", nil, 2, 999)
	require.NoError(t, err)
	assert.Equal(t, maxSearchSize, repo.gotLimit, "size 999 截断到上限")
	assert.Equal(t, maxSearchSize, repo.gotOffset, "page=2,size=50 → offset=50")
}

// Search：page/size<=0 → 归一为 1 / 默认
func TestSearch_Defaults(t *testing.T) {
	repo := &stubRepo{}
	_, err := newSvc(stubEmbedder{vec: []float32{1}}, repo).Search(context.Background(), "go", nil, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, repo.gotOffset, "page 归一为 1 → offset 0")
	assert.Equal(t, defaultSearchSize, repo.gotLimit)
}

// Search：embed 失败 → 返回错误（搜索无向量无法进行，非降级）
func TestSearch_EmbedErr(t *testing.T) {
	repo := &stubRepo{}
	_, err := newSvc(stubEmbedder{err: errors.New("embed down")}, repo).Search(context.Background(), "go", nil, 1, 10)
	require.Error(t, err)
	assert.False(t, repo.searchCalled)
}

// Index：embed 失败 → 降级 nil，不触达 repo
func TestIndex_EmbedFail_Degrade(t *testing.T) {
	repo := &stubRepo{}
	err := newSvc(stubEmbedder{err: errors.New("embed down")}, repo).Index(context.Background(), domain.Article{Id: 1, Title: "go"})
	require.NoError(t, err)
	assert.False(t, repo.indexCalled)
}

// Index：写 ES 失败 → 降级 nil（本次修复：写失败非致命）
func TestIndex_WriteFail_Degrade(t *testing.T) {
	repo := &stubRepo{indexErr: errors.New("es red")}
	err := newSvc(stubEmbedder{vec: []float32{1}}, repo).Index(context.Background(), domain.Article{Id: 1, Title: "go"})
	require.NoError(t, err, "写入失败应降级返回 nil")
	assert.True(t, repo.indexCalled)
}

// Remove：文档不存在 → 幂等 nil
func TestRemove_Idempotent(t *testing.T) {
	repo := &stubRepo{removeErr: errs.ErrSearchDocNotFound}
	err := newSvc(stubEmbedder{}, repo).Remove(context.Background(), 1)
	assert.NoError(t, err)
}

// Remove：其他错误 → 传播
func TestRemove_OtherErr(t *testing.T) {
	repo := &stubRepo{removeErr: errors.New("es down")}
	err := newSvc(stubEmbedder{}, repo).Remove(context.Background(), 1)
	assert.Error(t, err)
}

// RecommendTags：空文本 → nil，不触达 embed/repo
func TestRecommendTags_EmptyText(t *testing.T) {
	repo := &stubRepo{}
	tags, err := newSvc(stubEmbedder{}, repo).RecommendTags(context.Background(), "  ", "  ", 5)
	require.NoError(t, err)
	assert.Empty(t, tags)
	assert.False(t, repo.recCalled)
}

// RecommendTags：embed 失败 → 降级 nil（作者仍可手动打标签），不触达 repo
func TestRecommendTags_EmbedFail_Degrade(t *testing.T) {
	repo := &stubRepo{}
	tags, err := newSvc(stubEmbedder{err: errors.New("embed down")}, repo).RecommendTags(context.Background(), "go", "channel", 5)
	require.NoError(t, err)
	assert.Empty(t, tags)
	assert.False(t, repo.recCalled)
}
