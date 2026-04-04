package ai

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

// TimeoutFailoverClient 粘性主 provider + 连续故障计数切换
// 只用当前主 provider，连续故障达阈值时 CAS 切换到下一个
type TimeoutFailoverClient struct {
	clients   []LLMClient
	idx       int32 // 当前主 provider 下标
	cnt       int32 // 连续故障计数
	threshold int32 // 触发切换的故障阈值
	l         logger.LoggerX
}

func NewTimeoutFailoverClient(clients []LLMClient, threshold int32, l logger.LoggerX) LLMClient {
	if len(clients) == 0 {
		panic("LLM 提供方列表不能为空")
	}
	if threshold <= 0 {
		threshold = 3
	}
	return &TimeoutFailoverClient{
		clients:   clients,
		threshold: threshold,
		l:         l,
	}
}

func (t *TimeoutFailoverClient) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error) {
	idx := atomic.LoadInt32(&t.idx)
	cnt := atomic.LoadInt32(&t.cnt)

	// 连续故障达阈值，CAS 切换主 provider
	if cnt >= t.threshold {
		newIdx := (idx + 1) % int32(len(t.clients))
		if atomic.CompareAndSwapInt32(&t.idx, idx, newIdx) {
			atomic.StoreInt32(&t.cnt, 0)
			t.l.Warn("LLM 主动切换提供方",
				logger.Int32("consecutiveFails", cnt),
				logger.Int32("from", idx),
				logger.Int32("to", newIdx))
			idx = newIdx
		} else {
			idx = atomic.LoadInt32(&t.idx)
		}
	}

	ch, err := t.clients[idx].ChatStream(ctx, messages, tools)
	switch {
	case err == nil:
		atomic.StoreInt32(&t.cnt, 0)
		return ch, nil
	case errors.Is(err, context.Canceled):
		// 用户取消不计入故障
		return nil, err
	case errors.Is(err, context.DeadlineExceeded):
		// 超时计入故障
		atomic.AddInt32(&t.cnt, 1)
		t.l.Warn("LLM 提供方超时",
			logger.Int32("providerIdx", idx),
			logger.Error(err))
		return nil, err
	default:
		// 只有网络/服务端错误才计入故障，业务错误切 provider 也没用
		if t.isCriticalError(err) {
			atomic.AddInt32(&t.cnt, 1)
			t.l.Error("LLM 提供方关键错误",
				logger.Int32("providerIdx", idx),
				logger.Error(err))
		} else {
			t.l.Warn("LLM 提供方调用失败",
				logger.Int32("providerIdx", idx),
				logger.Error(err))
		}
		return nil, err
	}
}

func (t *TimeoutFailoverClient) isCriticalError(err error) bool {
	msg := strings.ToUpper(err.Error())
	criticalKeywords := []string{
		// 网络层
		"EOF", "REFUSED", "RESET", "BROKEN PIPE",
		"NO SUCH HOST", "I/O TIMEOUT", "TLS HANDSHAKE",
		// HTTP 5xx — 匹配 openai.go 输出格式 "STATUS=5xx"
		"STATUS=500", "STATUS=502", "STATUS=503", "STATUS=504", "STATUS=529",
		// LLM API 特有
		"RATE LIMIT", "TOO MANY REQUESTS", // 429
		"INSUFFICIENT_QUOTA", "BILLING", // 额度耗尽
		"MODEL_NOT_AVAILABLE", "OVERLOADED", // 模型不可用/过载
	}
	for _, key := range criticalKeywords {
		if strings.Contains(msg, key) {
			return true
		}
	}
	return false
}
