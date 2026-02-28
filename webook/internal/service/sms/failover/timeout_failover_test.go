package failover

import (
	"context"
	"errors"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/service/sms"
	smsmocks "gitee.com/train-cloud/geektime-basic-go/internal/service/sms/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestTimeoutFailoverSmsService_Send(t *testing.T) {
	testCases := []struct {
		name   string
		mock   func(ctrl *gomock.Controller) []sms.SmsService
		repeat int

		// 阈值设为 3
		threshold int32

		wantIdx int32
		wantCnt int32
		wantErr error
	}{
		{
			name:   "连续超时触发切换",
			repeat: 4,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				// 预期：前 3 次都在 s0，且都返回超时
				// 第 4 次应该切换到 s1
				gomock.InOrder(
					s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(context.DeadlineExceeded).Times(3),
					s1.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(nil).Times(1),
				)
				return []sms.SmsService{s0, s1}
			},
			threshold: 3,

			wantIdx: 1,
			wantCnt: 0,
			wantErr: nil,
		},
		{
			name:   "中间成功一次重置计数器",
			repeat: 5,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				// 1. 两次超时
				// 2. 一次成功（此时计数器应归零）
				// 3. 再两次超时（总共 4 次超时了，但因为不连续，不该切换）
				gomock.InOrder(
					s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(context.DeadlineExceeded).Times(2),
					s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(nil).Times(1),
					s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(context.DeadlineExceeded).Times(2),
				)

				return []sms.SmsService{s0, s1}
			},
			threshold: 3,

			wantIdx: 0,
			wantCnt: 2,
			wantErr: context.DeadlineExceeded,
		},
		{
			name:   "客户端主动取消",
			repeat: 10,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(context.Canceled).Times(10)
				return []sms.SmsService{s0}
			},
			threshold: 3,

			wantIdx: 0,
			wantCnt: 0,
			wantErr: context.Canceled,
		},
		{
			name:   "客户端列表为空",
			repeat: 1,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				return []sms.SmsService{}
			},
		},
		{
			name:   "超时阈值错误",
			repeat: 1,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return []sms.SmsService{s0}
			},
			threshold: 0,

			wantIdx: 0,
			wantCnt: 0,
			wantErr: nil,
		},
		{
			name:   "遇到熔断错误触发切换",
			repeat: 4,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				s1 := smsmocks.NewMockSmsService(ctrl)
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("EOF")).Times(3)
				s1.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("EOF")).Times(1)
				return []sms.SmsService{s0, s1}
			},
			threshold: 3,

			wantIdx: 1,
			wantCnt: 1,
			wantErr: errors.New("EOF"),
		},
		{
			name:   "遇到非严重错误不触发切换",
			repeat: 10,
			mock: func(ctrl *gomock.Controller) []sms.SmsService {
				s0 := smsmocks.NewMockSmsService(ctrl)
				// 业务报错（比如手机号格式不对），不计入切换计数
				s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("invalid phone number")).Times(10)
				return []sms.SmsService{s0}
			},
			threshold: 3,

			wantIdx: 0,
			wantCnt: 0,
			wantErr: errors.New("invalid phone number"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svcs := tc.mock(ctrl)
			if tc.name == "客户端列表为空" {
				assert.PanicsWithValue(t, "短信服务商列表不能为空", func() {
					NewTimeoutFailoverSmsService(svcs, tc.threshold)
				})
				return
			}

			svc := NewTimeoutFailoverSmsService(svcs, tc.threshold).(*TimeoutFailoverSmsService)

			// 为了测试连续性，我们需要多次调用
			var err error
			// 这里的循环次数要涵盖 mock 里的预期次数
			for i := 0; i < tc.repeat; i++ {
				err = svc.Send(context.Background(), "tpl", []string{"123"}, "188...")
			}
			assert.Equal(t, tc.wantIdx, svc.idx)
			assert.Equal(t, tc.wantCnt, svc.cnt)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}
func TestTimeoutFailoverSmsService_CAS_Else(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s0 := smsmocks.NewMockSmsService(ctrl)
	s1 := smsmocks.NewMockSmsService(ctrl)
	svcs := []sms.SmsService{s0, s1}

	//模拟已经达到触发点
	svc := &TimeoutFailoverSmsService{
		svcs:      svcs,
		idx:       0,
		cnt:       1,
		threshold: 1,
	}

	//断点测试可能会触发并发
	//预期：只有一个 s1 会被调用（因为切换后 idx 变成 1）
	//由于并发，我们预期两次调用都应该打到 s1 上
	concurrency := 100
	s0.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0).MaxTimes(0)
	s1.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(concurrency)

	//并发调用
	runtime.GOMAXPROCS(2)
	var wg sync.WaitGroup
	//就位
	start := make(chan struct{})
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			<-start //等待
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
			_ = svc.Send(context.Background(), "tpl", []string{"args"}, "188...")
		}()
	}

	//同时放行
	close(start)

	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.idx))
	assert.Equal(t, int32(0), atomic.LoadInt32(&svc.cnt))
}
