package embedding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/service/ai/embedding"
	embmocks "github.com/webook/internal/service/ai/embedding/mocks"
	"github.com/webook/pkg/logger"
)

func TestFailoverClient_Embed(t *testing.T) {
	wantVec := []float32{0.1, 0.2, 0.3}

	t.Run("第一个成功直接返回", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c0 := embmocks.NewMockEmbeddingClient(ctrl)
		c0.EXPECT().Embed(gomock.Any(), "hello").Return(wantVec, nil)

		f := embedding.NewFailoverClient([]embedding.EmbeddingClient{c0}, logger.NewNopLogger())
		vec, err := f.Embed(context.Background(), "hello")
		require.NoError(t, err)
		assert.Equal(t, wantVec, vec)
	})

	t.Run("第一个失败_第二个成功_降级", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c0 := embmocks.NewMockEmbeddingClient(ctrl)
		c1 := embmocks.NewMockEmbeddingClient(ctrl)
		c0.EXPECT().Embed(gomock.Any(), "hello").Return(nil, errors.New("connection refused"))
		c1.EXPECT().Embed(gomock.Any(), "hello").Return(wantVec, nil)

		f := embedding.NewFailoverClient([]embedding.EmbeddingClient{c0, c1}, logger.NewNopLogger())
		vec, err := f.Embed(context.Background(), "hello")
		require.NoError(t, err)
		assert.Equal(t, wantVec, vec)
	})

	t.Run("全部失败", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c0 := embmocks.NewMockEmbeddingClient(ctrl)
		c1 := embmocks.NewMockEmbeddingClient(ctrl)
		c0.EXPECT().Embed(gomock.Any(), "hello").Return(nil, errors.New("fail-0"))
		c1.EXPECT().Embed(gomock.Any(), "hello").Return(nil, errors.New("fail-1"))

		f := embedding.NewFailoverClient([]embedding.EmbeddingClient{c0, c1}, logger.NewNopLogger())
		_, err := f.Embed(context.Background(), "hello")
		assert.ErrorContains(t, err, "所有 Embedding 提供方均失败")
	})

	t.Run("context_Canceled_立即返回", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		c0 := embmocks.NewMockEmbeddingClient(ctrl)
		c1 := embmocks.NewMockEmbeddingClient(ctrl)
		c0.EXPECT().Embed(gomock.Any(), "hello").Return(nil, context.Canceled)

		f := embedding.NewFailoverClient([]embedding.EmbeddingClient{c0, c1}, logger.NewNopLogger())
		_, err := f.Embed(context.Background(), "hello")
		assert.ErrorIs(t, err, context.Canceled)
	})
}
