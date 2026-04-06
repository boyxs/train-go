package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	repomocks "gitee.com/train-cloud/geektime-basic-go/internal/repository/mocks"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/ai"
	aimocks "gitee.com/train-cloud/geektime-basic-go/internal/service/ai/mocks"
	svcmocks "gitee.com/train-cloud/geektime-basic-go/internal/service/mocks"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestChatService_SendMessage(t *testing.T) {
	testCases := []struct {
		name    string
		uid     int64
		convId  int64
		content string
		mock    func(ctrl *gomock.Controller) (repository.ConversationRepository, repository.MessageRepository, ai.LLMClient, ArticleSearchService)
		wantErr error
	}{
		{
			name:    "对话不存在",
			uid:     1,
			convId:  999,
			content: "hello",
			mock: func(ctrl *gomock.Controller) (repository.ConversationRepository, repository.MessageRepository, ai.LLMClient, ArticleSearchService) {
				convRepo := repomocks.NewMockConversationRepository(ctrl)
				msgRepo := repomocks.NewMockMessageRepository(ctrl)
				llm := aimocks.NewMockLLMClient(ctrl)
				search := svcmocks.NewMockArticleSearchService(ctrl)
				convRepo.EXPECT().Find(gomock.Any(), int64(1), int64(999)).
					Return(domain.Conversation{}, repository.ErrRecordNotFound)
				return convRepo, msgRepo, llm, search
			},
			wantErr: ErrConversationNotFound,
		},
		{
			name:    "消息过长",
			uid:     1,
			convId:  1,
			content: string(make([]rune, 2001)),
			mock: func(ctrl *gomock.Controller) (repository.ConversationRepository, repository.MessageRepository, ai.LLMClient, ArticleSearchService) {
				convRepo := repomocks.NewMockConversationRepository(ctrl)
				msgRepo := repomocks.NewMockMessageRepository(ctrl)
				llm := aimocks.NewMockLLMClient(ctrl)
				search := svcmocks.NewMockArticleSearchService(ctrl)
				convRepo.EXPECT().Find(gomock.Any(), int64(1), int64(1)).
					Return(domain.Conversation{Id: 1, UserId: 1}, nil)
				return convRepo, msgRepo, llm, search
			},
			wantErr: ErrMessageTooLong,
		},
		{
			name:    "LLM 调用失败",
			uid:     1,
			convId:  1,
			content: "hello",
			mock: func(ctrl *gomock.Controller) (repository.ConversationRepository, repository.MessageRepository, ai.LLMClient, ArticleSearchService) {
				convRepo := repomocks.NewMockConversationRepository(ctrl)
				msgRepo := repomocks.NewMockMessageRepository(ctrl)
				llm := aimocks.NewMockLLMClient(ctrl)
				search := svcmocks.NewMockArticleSearchService(ctrl)
				convRepo.EXPECT().Find(gomock.Any(), int64(1), int64(1)).
					Return(domain.Conversation{Id: 1, UserId: 1}, nil)
				msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
					Return(domain.Message{Id: 1}, nil)
				msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), maxHistoryRounds*2).
					Return([]domain.Message{{Role: "user", Content: "hello"}}, nil)
				// RAG 检索（正常返回空）
				search.EXPECT().Search(gomock.Any(), "hello", 1, ragTopK).
					Return(nil, int64(0), nil)
				llm.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("all providers down"))
				return convRepo, msgRepo, llm, search
			},
			wantErr: errors.New("调用 LLM 失败"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			convRepo, msgRepo, llm, search := tc.mock(ctrl)
			svc := NewChatService(convRepo, msgRepo, llm, search, logger.NewNopLogger())

			_, err := svc.SendMessage(context.Background(), tc.uid, tc.convId, tc.content)
			if tc.wantErr != nil {
				assert.ErrorContains(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChatService_ForwardStream_Done(t *testing.T) {
	// LLM 正常完成：delta + done → 保存完整回复
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	convRepo := repomocks.NewMockConversationRepository(ctrl)

	// 保存 AI 回复
	msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, msg domain.Message) (domain.Message, error) {
			assert.Equal(t, "assistant", msg.Role)
			assert.Equal(t, "你好世界", msg.Content)
			return domain.Message{Id: 100}, nil
		})
	// autoTitle: ListRecent 返回 2 条（首轮对话）
	msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), 3).
		Return([]domain.Message{{}, {}}, nil)
	// UpdateTitle
	convRepo.EXPECT().UpdateTitle(gomock.Any(), int64(1), int64(1), gomock.Any()).Return(nil)

	svc := &chatService{
		convRepo: convRepo,
		msgRepo:  msgRepo,
		l:        logger.NewNopLogger(),
	}

	llmCh := make(chan ai.StreamChunk, 3)
	llmCh <- ai.StreamChunk{Type: "text", Content: "你好"}
	llmCh <- ai.StreamChunk{Type: "text", Content: "世界"}
	llmCh <- ai.StreamChunk{Type: "done"}
	close(llmCh)

	eventCh := make(chan domain.ChatEvent, 10)
	ctx := context.Background()
	svc.forwardStream(ctx, 1, 1, llmCh, eventCh)

	// 收集事件
	var events []domain.ChatEvent
	for e := range eventCh {
		events = append(events, e)
	}

	assert.Len(t, events, 3) // 2 delta + 1 done
	assert.Equal(t, "delta", events[0].Type)
	assert.Equal(t, "delta", events[1].Type)
	assert.Equal(t, "done", events[2].Type)
}

func TestChatService_ForwardStream_CtxCancel(t *testing.T) {
	// 前端断开（ctx 取消）→ 保存已有部分回复，goroutine 退出不泄漏
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	convRepo := repomocks.NewMockConversationRepository(ctrl)

	// 部分回复应被保存
	msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, msg domain.Message) (domain.Message, error) {
			assert.Equal(t, "assistant", msg.Role)
			assert.Equal(t, "部分", msg.Content)
			return domain.Message{Id: 50}, nil
		})
	// autoTitle
	msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), 3).
		Return([]domain.Message{{}, {}, {}}, nil) // 3 条 → 非首轮，不更新标题

	svc := &chatService{
		convRepo: convRepo,
		msgRepo:  msgRepo,
		l:        logger.NewNopLogger(),
	}

	llmCh := make(chan ai.StreamChunk, 2)
	llmCh <- ai.StreamChunk{Type: "text", Content: "部分"}
	// 不发 done，模拟卡住

	eventCh := make(chan domain.ChatEvent, 10)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.forwardStream(ctx, 1, 1, llmCh, eventCh)
		close(done)
	}()

	// 等第一个 delta 被消费
	e := <-eventCh
	assert.Equal(t, "delta", e.Type)

	// 模拟前端断开
	cancel()

	// 验证 goroutine 退出（不泄漏）
	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("forwardStream goroutine 未退出，泄漏")
	}
}

func TestChatService_ForwardStream_Error(t *testing.T) {
	// LLM 发送 error 事件 → 保存已有部分回复
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	convRepo := repomocks.NewMockConversationRepository(ctrl)

	// 有部分内容，应保存
	msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, msg domain.Message) (domain.Message, error) {
			assert.Equal(t, "一些内容", msg.Content)
			return domain.Message{Id: 60}, nil
		})
	msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), 3).
		Return([]domain.Message{{}, {}, {}}, nil)

	svc := &chatService{
		convRepo: convRepo,
		msgRepo:  msgRepo,
		l:        logger.NewNopLogger(),
	}

	llmCh := make(chan ai.StreamChunk, 3)
	llmCh <- ai.StreamChunk{Type: "text", Content: "一些内容"}
	llmCh <- ai.StreamChunk{Type: "error", Content: "provider crashed"}
	close(llmCh)

	eventCh := make(chan domain.ChatEvent, 10)
	svc.forwardStream(context.Background(), 1, 1, llmCh, eventCh)

	var events []domain.ChatEvent
	for e := range eventCh {
		events = append(events, e)
	}

	assert.Len(t, events, 2) // 1 delta + 1 error
	assert.Equal(t, "delta", events[0].Type)
	assert.Equal(t, "error", events[1].Type)
}

func TestChatService_ForwardStream_ErrorNoContent(t *testing.T) {
	// LLM 立即发 error，无内容 → 不保存空回复
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	convRepo := repomocks.NewMockConversationRepository(ctrl)
	// 不应调用 Insert（空内容不保存）

	svc := &chatService{
		convRepo: convRepo,
		msgRepo:  msgRepo,
		l:        logger.NewNopLogger(),
	}

	llmCh := make(chan ai.StreamChunk, 1)
	llmCh <- ai.StreamChunk{Type: "error", Content: "immediate failure"}
	close(llmCh)

	eventCh := make(chan domain.ChatEvent, 10)
	svc.forwardStream(context.Background(), 1, 1, llmCh, eventCh)

	var events []domain.ChatEvent
	for e := range eventCh {
		events = append(events, e)
	}

	assert.Len(t, events, 1)
	assert.Equal(t, "error", events[0].Type)
}

func TestChatService_SendMessage_RAGWithArticles(t *testing.T) {
	// 检索到文章 → LLM 收到的 messages 中应包含文章上下文
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	convRepo := repomocks.NewMockConversationRepository(ctrl)
	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	llm := aimocks.NewMockLLMClient(ctrl)
	search := svcmocks.NewMockArticleSearchService(ctrl)

	convRepo.EXPECT().Find(gomock.Any(), int64(1), int64(1)).
		Return(domain.Conversation{Id: 1, UserId: 1}, nil)
	msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		Return(domain.Message{Id: 1}, nil)
	msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), maxHistoryRounds*2).
		Return([]domain.Message{{Role: "user", Content: "Go并发怎么写"}}, nil)

	// RAG 返回 2 篇文章
	search.EXPECT().Search(gomock.Any(), "Go并发怎么写", 1, ragTopK).
		Return([]domain.Article{
			{Id: 10, Title: "Go 并发入门", Abstract: "goroutine 和 channel", Author: domain.Author{Name: "张三"}},
			{Id: 20, Title: "并发模式", Abstract: "常见并发设计模式", Author: domain.Author{Name: "李四"}},
		}, int64(2), nil)

	// 验证 LLM 收到的 messages 包含 RAG 上下文
	llm.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, messages []ai.ChatMessage, tools []ai.Tool) (<-chan ai.StreamChunk, error) {
			// messages[0] = system prompt, messages[1] = RAG context, messages[2] = user msg
			assert.GreaterOrEqual(t, len(messages), 3)
			ragMsg := messages[1]
			assert.Equal(t, "system", ragMsg.Role)
			assert.Contains(t, ragMsg.Content, "[Go 并发入门](/article/10)")
			assert.Contains(t, ragMsg.Content, "goroutine 和 channel")
			assert.Contains(t, ragMsg.Content, "[并发模式](/article/20)")
			assert.Contains(t, ragMsg.Content, "常见并发设计模式")

			ch := make(chan ai.StreamChunk, 1)
			ch <- ai.StreamChunk{Type: "done"}
			close(ch)
			return ch, nil
		})

	svc := NewChatService(convRepo, msgRepo, llm, search, logger.NewNopLogger())
	_, err := svc.SendMessage(context.Background(), 1, 1, "Go并发怎么写")
	assert.NoError(t, err)
}

func TestChatService_SendMessage_RAGNoResults(t *testing.T) {
	// 检索无结果 → prompt 中不应出现 RAG 上下文消息
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	convRepo := repomocks.NewMockConversationRepository(ctrl)
	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	llm := aimocks.NewMockLLMClient(ctrl)
	search := svcmocks.NewMockArticleSearchService(ctrl)

	convRepo.EXPECT().Find(gomock.Any(), int64(1), int64(1)).
		Return(domain.Conversation{Id: 1, UserId: 1}, nil)
	msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		Return(domain.Message{Id: 1}, nil)
	msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), maxHistoryRounds*2).
		Return([]domain.Message{{Role: "user", Content: "你好"}}, nil)

	// 检索无结果
	search.EXPECT().Search(gomock.Any(), "你好", 1, ragTopK).
		Return(nil, int64(0), nil)

	// 验证只有 system prompt + 历史消息，无 RAG 上下文
	llm.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, messages []ai.ChatMessage, tools []ai.Tool) (<-chan ai.StreamChunk, error) {
			assert.Len(t, messages, 2) // system + user
			assert.Equal(t, "system", messages[0].Role)
			assert.Equal(t, "user", messages[1].Role)

			ch := make(chan ai.StreamChunk, 1)
			ch <- ai.StreamChunk{Type: "done"}
			close(ch)
			return ch, nil
		})

	svc := NewChatService(convRepo, msgRepo, llm, search, logger.NewNopLogger())
	_, err := svc.SendMessage(context.Background(), 1, 1, "你好")
	assert.NoError(t, err)
}

func TestChatService_SendMessage_RAGSearchFail(t *testing.T) {
	// 检索失败 → 静默降级，不影响对话
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	convRepo := repomocks.NewMockConversationRepository(ctrl)
	msgRepo := repomocks.NewMockMessageRepository(ctrl)
	llm := aimocks.NewMockLLMClient(ctrl)
	search := svcmocks.NewMockArticleSearchService(ctrl)

	convRepo.EXPECT().Find(gomock.Any(), int64(1), int64(1)).
		Return(domain.Conversation{Id: 1, UserId: 1}, nil)
	msgRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		Return(domain.Message{Id: 1}, nil)
	msgRepo.EXPECT().ListRecent(gomock.Any(), int64(1), maxHistoryRounds*2).
		Return([]domain.Message{{Role: "user", Content: "文章推荐"}}, nil)

	// 检索失败
	search.EXPECT().Search(gomock.Any(), "文章推荐", 1, ragTopK).
		Return(nil, int64(0), errors.New("ES connection refused"))

	// 即使检索失败，LLM 仍被调用（降级为无 RAG）
	llm.EXPECT().ChatStream(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, messages []ai.ChatMessage, tools []ai.Tool) (<-chan ai.StreamChunk, error) {
			assert.Len(t, messages, 2) // system + user，无 RAG
			ch := make(chan ai.StreamChunk, 1)
			ch <- ai.StreamChunk{Type: "done"}
			close(ch)
			return ch, nil
		})

	svc := NewChatService(convRepo, msgRepo, llm, search, logger.NewNopLogger())
	_, err := svc.SendMessage(context.Background(), 1, 1, "文章推荐")
	assert.NoError(t, err)
}

func TestInjectArticleContext(t *testing.T) {
	svc := &chatService{}
	messages := []ai.ChatMessage{
		{Role: "system", Content: "你是助手"},
		{Role: "user", Content: "有什么好文章"},
	}
	articles := []domain.Article{
		{Id: 42, Title: "Go 测试指南", Abstract: "表格驱动测试", Author: domain.Author{Name: "王五"}},
	}

	result := svc.injectArticleContext(messages, articles)

	// system + RAG context + user = 3 条
	assert.Len(t, result, 3)
	assert.Equal(t, "system", result[0].Role)
	assert.Equal(t, "system", result[1].Role)
	assert.Contains(t, result[1].Content, "[Go 测试指南](/article/42)")
	assert.Contains(t, result[1].Content, "王五")
	assert.Contains(t, result[1].Content, "表格驱动测试")
	assert.Equal(t, "user", result[2].Role)
}
