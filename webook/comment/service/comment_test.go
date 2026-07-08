package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/boyxs/train-go/webook/comment/domain"
	"github.com/boyxs/train-go/webook/comment/errs"
	repomocks "github.com/boyxs/train-go/webook/comment/repository/mocks"
	"github.com/boyxs/train-go/webook/pkg/logger"
	limitmocks "github.com/boyxs/train-go/webook/pkg/ratelimit/mocks"
	sensitivemocks "github.com/boyxs/train-go/webook/pkg/sensitive/mocks"
)

func TestCommentService_Create(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(*repomocks.MockCommentRepository, *sensitivemocks.MockFilter, *limitmocks.MockLimiter)
		content string
		wantErr error
	}{
		{
			name: "正常发表",
			setup: func(repo *repomocks.MockCommentRepository, filter *sensitivemocks.MockFilter, limiter *limitmocks.MockLimiter) {
				filter.EXPECT().Match("这是正常评论").Return(false)
				limiter.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(false, nil)
				repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.Comment{Id: 1, Content: "这是正常评论"}, nil)
			},
			content: "这是正常评论",
			wantErr: nil,
		},
		{
			name: "敏感词拦截，不落库",
			setup: func(repo *repomocks.MockCommentRepository, filter *sensitivemocks.MockFilter, limiter *limitmocks.MockLimiter) {
				filter.EXPECT().Match("敏感内容").Return(true)
				// 不应再调 limiter / repo
			},
			content: "敏感内容",
			wantErr: errs.ErrSensitiveContent,
		},
		{
			name: "限流拦截",
			setup: func(repo *repomocks.MockCommentRepository, filter *sensitivemocks.MockFilter, limiter *limitmocks.MockLimiter) {
				filter.EXPECT().Match(gomock.Any()).Return(false)
				limiter.EXPECT().Limit(gomock.Any(), gomock.Any()).Return(true, nil)
			},
			content: "正常但被限流",
			wantErr: errs.ErrRateLimited,
		},
		{
			name: "空内容直接拒绝",
			setup: func(repo *repomocks.MockCommentRepository, filter *sensitivemocks.MockFilter, limiter *limitmocks.MockLimiter) {
			},
			content: "   ",
			wantErr: errs.ErrContentEmpty,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockCommentRepository(ctrl)
			filter := sensitivemocks.NewMockFilter(ctrl)
			limiter := limitmocks.NewMockLimiter(ctrl)
			c.setup(repo, filter, limiter)
			svc := NewCommentService(repo, filter, limiter, logger.NewNopLogger())
			_, err := svc.Create(context.Background(), domain.Comment{
				Biz: "article", BizId: 1, UserId: 100, Content: c.content,
			})
			assert.ErrorIs(t, err, c.wantErr)
		})
	}
}

func TestCommentService_Delete(t *testing.T) {
	t.Run("作者删除成功", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := repomocks.NewMockCommentRepository(ctrl)
		repo.EXPECT().Delete(gomock.Any(), int64(1), int64(100)).Return(true, nil)
		svc := NewCommentService(repo, sensitivemocks.NewMockFilter(ctrl), limitmocks.NewMockLimiter(ctrl), logger.NewNopLogger())
		err := svc.Delete(context.Background(), 1, 100)
		require.NoError(t, err)
	})
	t.Run("非作者拒绝", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := repomocks.NewMockCommentRepository(ctrl)
		repo.EXPECT().Delete(gomock.Any(), int64(1), int64(999)).Return(false, nil)
		svc := NewCommentService(repo, sensitivemocks.NewMockFilter(ctrl), limitmocks.NewMockLimiter(ctrl), logger.NewNopLogger())
		err := svc.Delete(context.Background(), 1, 999)
		assert.ErrorIs(t, err, errs.ErrForbidden)
	})
}
