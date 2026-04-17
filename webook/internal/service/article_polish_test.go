package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	aimocks "github.com/webook/internal/service/ai/mocks"
)

func TestAIArticlePolishService_Polish(t *testing.T) {
	testCases := []struct {
		name      string
		title     string
		content   string
		mock      func(ctrl *gomock.Controller) *aimocks.MockLLMClient
		wantTitle string
		wantErr   error
	}{
		{
			name:    "正常润色",
			title:   "Go 并发",
			content: "goroutine 很好用",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				llm := aimocks.NewMockLLMClient(ctrl)
				llm.EXPECT().Chat(gomock.Any(), gomock.Any()).Return(
					`{"title":"Go 并发编程入门","abstract":"介绍 goroutine 核心概念","content":"Goroutine 是 Go 的核心并发原语，使用简单高效。"}`,
					nil,
				)
				return llm
			},
			wantTitle: "Go 并发编程入门",
		},
		{
			name:    "LLM 返回 markdown code block 包裹",
			title:   "测试",
			content: "内容",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				llm := aimocks.NewMockLLMClient(ctrl)
				llm.EXPECT().Chat(gomock.Any(), gomock.Any()).Return(
					"```json\n{\"title\":\"测试标题\",\"abstract\":\"测试摘要\",\"content\":\"测试内容\"}\n```",
					nil,
				)
				return llm
			},
			wantTitle: "测试标题",
		},
		{
			name:    "LLM 返回不完整 JSON",
			title:   "测试",
			content: "内容",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				llm := aimocks.NewMockLLMClient(ctrl)
				llm.EXPECT().Chat(gomock.Any(), gomock.Any()).Return(
					`{"title":"","abstract":"摘要","content":""}`,
					nil,
				)
				return llm
			},
			wantErr: errors.New("润色结果不完整"),
		},
		{
			name:    "LLM 返回非 JSON",
			title:   "测试",
			content: "内容",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				llm := aimocks.NewMockLLMClient(ctrl)
				llm.EXPECT().Chat(gomock.Any(), gomock.Any()).Return("这不是JSON", nil)
				return llm
			},
			wantErr: errors.New("解析润色结果失败"),
		},
		{
			name:    "LLM 调用失败",
			title:   "测试",
			content: "内容",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				llm := aimocks.NewMockLLMClient(ctrl)
				llm.EXPECT().Chat(gomock.Any(), gomock.Any()).Return("", errors.New("network error"))
				return llm
			},
			wantErr: errors.New("LLM 调用失败"),
		},
		{
			name:    "title 为空",
			title:   "",
			content: "有内容",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				return aimocks.NewMockLLMClient(ctrl)
			},
			wantErr: ErrPolishEmptyTitle,
		},
		{
			name:    "content 为空",
			title:   "有标题",
			content: "",
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				return aimocks.NewMockLLMClient(ctrl)
			},
			wantErr: ErrPolishEmptyContent,
		},
		{
			name:    "content 超长",
			title:   "标题",
			content: string(make([]rune, polishMaxContentLen+1)),
			mock: func(ctrl *gomock.Controller) *aimocks.MockLLMClient {
				return aimocks.NewMockLLMClient(ctrl)
			},
			wantErr: ErrPolishContentTooLong,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			llm := tc.mock(ctrl)
			svc := NewAIArticlePolishService(llm)
			result, err := svc.Polish(context.Background(), tc.title, tc.content)
			if tc.wantErr != nil {
				assert.ErrorContains(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantTitle, result.Title)
				assert.NotEmpty(t, result.Abstract)
				assert.NotEmpty(t, result.Content)
			}
		})
	}
}
