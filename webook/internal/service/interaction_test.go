package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/webook/internal/repository"
	repomocks "github.com/webook/internal/repository/mocks"
)

func TestInternalInteractionService_FindUserLiked(t *testing.T) {
	const (
		uid = int64(1)
		biz = "comment"
	)
	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) repository.InteractionRepository
		bizIds  []int64
		want    map[int64]bool
		wantErr error
	}{
		{
			name: "空 bizIds 直接返回空 map，不调 repo",
			mock: func(ctrl *gomock.Controller) repository.InteractionRepository {
				// 不设置任何 EXPECT，调用了即 fail
				return repomocks.NewMockInteractionRepository(ctrl)
			},
			bizIds: []int64{},
			want:   map[int64]bool{},
		},
		{
			name: "用户未赞任何，返回空 map（非 nil）",
			mock: func(ctrl *gomock.Controller) repository.InteractionRepository {
				repo := repomocks.NewMockInteractionRepository(ctrl)
				repo.EXPECT().FindLikedBizIds(gomock.Any(), uid, biz, []int64{1, 2, 3}).
					Return([]int64{}, nil)
				return repo
			},
			bizIds: []int64{1, 2, 3},
			want:   map[int64]bool{},
		},
		{
			name: "部分点赞，只含已赞=true",
			mock: func(ctrl *gomock.Controller) repository.InteractionRepository {
				repo := repomocks.NewMockInteractionRepository(ctrl)
				repo.EXPECT().FindLikedBizIds(gomock.Any(), uid, biz, []int64{1, 2, 3}).
					Return([]int64{1, 3}, nil)
				return repo
			},
			bizIds: []int64{1, 2, 3},
			want:   map[int64]bool{1: true, 3: true},
		},
		{
			name: "repo 报错透传",
			mock: func(ctrl *gomock.Controller) repository.InteractionRepository {
				repo := repomocks.NewMockInteractionRepository(ctrl)
				repo.EXPECT().FindLikedBizIds(gomock.Any(), uid, biz, []int64{1}).
					Return(nil, errors.New("db error"))
				return repo
			},
			bizIds:  []int64{1},
			wantErr: errors.New("db error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := NewInternalInteractionService(tc.mock(ctrl))
			got, err := svc.FindUserLiked(context.Background(), uid, biz, tc.bizIds)

			if tc.wantErr != nil {
				assert.Equal(t, tc.wantErr, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
