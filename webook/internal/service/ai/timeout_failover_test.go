package ai_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/webook/internal/service/ai"
	aimocks "github.com/webook/internal/service/ai/mocks"
	"github.com/webook/pkg/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestTimeoutFailoverClient_ChatStream(t *testing.T) {
	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) []ai.LLMClient
		wantErr string
	}{
		{
			name: "主 provider 成功",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(makeCh(), nil)
				return []ai.LLMClient{c0}
			},
		},
		{
			name: "业务错误不计入故障",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("invalid model"))
				return []ai.LLMClient{c0}
			},
			wantErr: "invalid model",
		},
		{
			name: "context.Canceled 不计入故障",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, context.Canceled)
				return []ai.LLMClient{c0}
			},
			wantErr: "context canceled",
		},
		{
			name: "context.DeadlineExceeded 计入故障",
			mock: func(ctrl *gomock.Controller) []ai.LLMClient {
				c0 := aimocks.NewMockLLMClient(ctrl)
				c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, context.DeadlineExceeded)
				return []ai.LLMClient{c0}
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			f := ai.NewTimeoutFailoverClient(tc.mock(ctrl), 3, logger.NewNopLogger())
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

func TestTimeoutFailoverClient_TimeoutTriggersSwitch(t *testing.T) {
	// 连续 3 次超时后，第 4 次应切换到 c1
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	c0 := aimocks.NewMockLLMClient(ctrl)
	c1 := aimocks.NewMockLLMClient(ctrl)

	gomock.InOrder(
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		// 第 4 次：阈值达到，切换到 c1
		c1.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makeCh(), nil),
	)

	f := ai.NewTimeoutFailoverClient([]ai.LLMClient{c0, c1}, 3, logger.NewNopLogger())

	for i := 0; i < 3; i++ {
		_, err := f.ChatStream(context.Background(), msgs, nil)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}

	ch, err := f.ChatStream(context.Background(), msgs, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestTimeoutFailoverClient_CriticalErrorTriggersSwitch(t *testing.T) {
	// 连续 3 次关键错误（网络/5xx）后切换
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	c0 := aimocks.NewMockLLMClient(ctrl)
	c1 := aimocks.NewMockLLMClient(ctrl)

	gomock.InOrder(
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("[deepseek] API error: status=502, body=bad gateway")),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("connection refused")),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("read: connection reset")),
		// 阈值达到，切换到 c1
		c1.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makeCh(), nil),
	)

	f := ai.NewTimeoutFailoverClient([]ai.LLMClient{c0, c1}, 3, logger.NewNopLogger())

	for i := 0; i < 3; i++ {
		_, err := f.ChatStream(context.Background(), msgs, nil)
		assert.Error(t, err)
	}

	ch, err := f.ChatStream(context.Background(), msgs, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestTimeoutFailoverClient_NonCriticalErrorNoSwitch(t *testing.T) {
	// 非关键错误（业务错误）不计入 cnt，不触发切换
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	c0 := aimocks.NewMockLLMClient(ctrl)
	c1 := aimocks.NewMockLLMClient(ctrl)

	gomock.InOrder(
		// 连续 4 次业务错误，不应切换
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("invalid model")),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("bad request: content too long")),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("invalid model")),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("invalid model")),
		// 第 5 次仍在 c0（没切换）
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makeCh(), nil),
	)

	f := ai.NewTimeoutFailoverClient([]ai.LLMClient{c0, c1}, 3, logger.NewNopLogger())

	for i := 0; i < 4; i++ {
		_, err := f.ChatStream(context.Background(), msgs, nil)
		assert.Error(t, err)
	}

	// 仍在 c0
	ch, err := f.ChatStream(context.Background(), msgs, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestTimeoutFailoverClient_SuccessResetsCount(t *testing.T) {
	// 故障 2 次（未达阈值 3）→ 成功 → 计数归零 → 再故障 2 次 → 仍用 c0
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	c0 := aimocks.NewMockLLMClient(ctrl)
	c1 := aimocks.NewMockLLMClient(ctrl)

	gomock.InOrder(
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		// 成功 → 清零
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makeCh(), nil),
		// 再超时 2 次，仍在 c0（未达阈值）
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.DeadlineExceeded),
		// c0 恢复
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makeCh(), nil),
	)

	f := ai.NewTimeoutFailoverClient([]ai.LLMClient{c0, c1}, 3, logger.NewNopLogger())

	// 超时 2 次
	for i := 0; i < 2; i++ {
		_, err := f.ChatStream(context.Background(), msgs, nil)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
	// 成功
	ch, err := f.ChatStream(context.Background(), msgs, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ch)

	// 再超时 2 次
	for i := 0; i < 2; i++ {
		_, err = f.ChatStream(context.Background(), msgs, nil)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
	// 仍在 c0，c0 恢复
	ch, err = f.ChatStream(context.Background(), msgs, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestTimeoutFailoverClient_CancelNotCountAsFailure(t *testing.T) {
	// 3 次取消后主 provider 不变
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgs := []ai.ChatMessage{{Role: "user", Content: "hi"}}

	c0 := aimocks.NewMockLLMClient(ctrl)
	c1 := aimocks.NewMockLLMClient(ctrl)

	gomock.InOrder(
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.Canceled),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.Canceled),
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, context.Canceled),
		// 第 4 次仍用 c0（没切换）
		c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(makeCh(), nil),
	)

	f := ai.NewTimeoutFailoverClient([]ai.LLMClient{c0, c1}, 3, logger.NewNopLogger())

	for i := 0; i < 3; i++ {
		_, err := f.ChatStream(context.Background(), msgs, nil)
		assert.ErrorIs(t, err, context.Canceled)
	}

	ch, err := f.ChatStream(context.Background(), msgs, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestTimeoutFailoverClient_Concurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c0 := aimocks.NewMockLLMClient(ctrl)
	c0.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(makeCh(), nil).Times(20)

	f := ai.NewTimeoutFailoverClient([]ai.LLMClient{c0}, 3, logger.NewNopLogger())

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
