package failover

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/webook/internal/service/sms"
	smsmocks "github.com/webook/internal/service/sms/mocks"
	"github.com/webook/pkg/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -source=../types.go -package=smsmocks -destination=../mocks/sms_mock.go
func TestFailoverSmsService_Send(t *testing.T) {
	testCases := []struct {
		name    string
		mock    func(ctrl *gomock.Controller) []sms.SmsService
		wantErr error
	}{
		{
			name: "没有可用的短信服务商",
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				return []sms.SmsService{}
			},
			wantErr: errors.New("没有可用的短信服务商"),
		}, {
			name: "第一个服务商成功",
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return []sms.SmsService{s0, s1}
			},
			wantErr: nil,
		},
		{
			name: "第一个失败，第二个成功（故障转移）",
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("network error"))
				s1.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return []sms.SmsService{s0, s1}
			},
			wantErr: nil,
		},
		{
			name: "全部失败",
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail"))
				s1.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail"))

				return []sms.SmsService{s0, s1}
			},
			wantErr: errors.New("轮询所有服务商均告失败"),
		},
		{
			name: "Context超时应立即停止",
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(context.DeadlineExceeded)
				return []sms.SmsService{s0, s1}
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := NewFailoverSmsService(tc.mock(ctrl), logger.NewNopLogger())

			err := svc.Send(context.Background(), "tpl", []string{"123"}, "13800138000")
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

// 并发测试：确保严格轮询在多协程下序号不冲突
func TestFailoverSmsService_Parallel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	numSvcs := 3
	numRequests := 30
	mockSvcs := make([]sms.SmsService, numSvcs)

	for i := 0; i < numSvcs; i++ {
		s := smsmocks.NewMockSmsService(ctrl)
		// 严格轮询下，30个请求平均分配给3个服务商，每个必得10次
		s.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(numRequests / numSvcs)
		mockSvcs[i] = s
	}

	svcInterface := NewFailoverSmsService(mockSvcs, logger.NewNopLogger())

	f, ok := svcInterface.(*FailoverSmsService)
	assert.True(t, ok)

	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			// 执行发送
			err := f.Send(context.Background(), "tpl", []string{"args"}, "138...")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// 检查 idx 的原子递增结果
	// 注意：idx 初始为 0，增加了 30 次，最终应该是 30
	finalIdx := atomic.LoadUint64(&f.idx)
	assert.Equal(t, uint64(numRequests), finalIdx)
}
