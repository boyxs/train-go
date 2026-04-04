package ai_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/ai"
	aimocks "gitee.com/train-cloud/geektime-basic-go/internal/service/ai/mocks"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func makeCh() <-chan ai.StreamChunk {
	ch := make(chan ai.StreamChunk, 1)
	ch <- ai.StreamChunk{Type: "done"}
	close(ch)
	return ch
}

func TestFailoverClient_ChatStream(t *testing.T) {
	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) []ai.LLMClient
		wantErr string
	}{
		{
			name: "第一个成功",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeCh(), nil)
				return []ai.LLMClient{c0}
			},
		},
		{
			name: "首个失败，第二个成功（故障转移）",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c1 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("down"))
				c1.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeCh(), nil)
				return []ai.LLMClient{c0, c1}
			},
		},
		{
			name: "全部失败",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c1 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("fail-0"))
				c1.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("fail-1"))
				return []ai.LLMClient{c0, c1}
			},
			wantErr: "轮询所有 LLM 提供方均失败",
		},
		{
			name: "context.Canceled 立即返回，不轮询",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c1 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, context.Canceled)
				return []ai.LLMClient{c0, c1}
			},
			wantErr: "context canceled",
		},
		{
			name: "context.DeadlineExceeded 立即返回，不轮询",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c1 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, context.DeadlineExceeded)
				return []ai.LLMClient{c0, c1}
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			f := ai.NewFailoverClient(tc.mock(ctrl), logger.NewNopLogger())
			ch, err := f.ChatStream(context.Background(), msgs, nil)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, ch)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ch)
			}
		})
	}
}

func TestFailoverClient_Concurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c0 := aimocks.NewMockLLMClient(ctrl)
	c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(makeCh(), nil).Times(20)

	f := ai.NewFailoverClient([]ai.LLMClient{c0}, logger.NewNopLogger())

	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}
	var wg sync.WaitGroup
	errs := make([]error, 20)
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = f.ChatStream(context.Background(), msgs, nil)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d failed", i)
	}
}
