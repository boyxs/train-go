package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/ai"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
)

const (
	maxMessageLength = 2000
	maxHistoryRounds = 20
	titleMaxRunes    = 20
	ragTopK          = 3
	maxToolRounds    = 5 // 工具调用最大轮次，防止无限循环
	systemPrompt     = `你是小微书平台的 AI 助手。你的职责是帮助用户解答平台使用问题、推荐文章内容。
规则：
1. 只回答与小微书平台相关的问题
2. 不回答涉及政治、暴力、色情的内容
3. 回答简洁友好，使用中文
4. 如果不确定，坦诚告知用户
5. 如果系统提供了相关文章，优先基于文章内容回答，并在回答中引用来源
6. 引用时直接复制系统提供的 Markdown 链接，不要自己编造链接
7. 如果提供的文章与问题无关，忽略它们，用自己的知识回答
8. 热门文章、用户收藏等实时数据每次必须重新调用对应工具获取，不能复用历史对话中出现过的数据`
)

var (
	ErrConversationNotFound = errors.New("对话不存在")
	ErrMessageTooLong       = errors.New("消息内容过长")
)

type ChatService interface {
	CreateConversation(ctx context.Context, uid int64) (domain.Conversation, error)
	ListConversations(ctx context.Context, uid int64) ([]domain.Conversation, error)
	DeleteConversation(ctx context.Context, uid int64, convId int64) error
	ListMessages(ctx context.Context, uid int64, convId int64, beforeId int64, limit int) ([]domain.Message, error)
	SendMessage(ctx context.Context, uid int64, convId int64, content string) (<-chan domain.ChatEvent, error)
	StopGeneration(ctx context.Context, uid int64, convId int64) error
}

type chatService struct {
	convRepo repository.ConversationRepository
	msgRepo  repository.MessageRepository
	llm      ai.LLMClient
	search   ArticleSearchService
	executor ToolExecutor
	l        logger.LoggerX
	cancel   sync.Map // convId -> context.CancelFunc
}

func NewChatService(
	convRepo repository.ConversationRepository,
	msgRepo repository.MessageRepository,
	llm ai.LLMClient,
	search ArticleSearchService,
	executor ToolExecutor,
	l logger.LoggerX,
) ChatService {
	return &chatService{
		convRepo: convRepo,
		msgRepo:  msgRepo,
		llm:      llm,
		search:   search,
		executor: executor,
		l:        l,
	}
}

func (s *chatService) CreateConversation(ctx context.Context, uid int64) (domain.Conversation, error) {
	return s.convRepo.Create(ctx, domain.Conversation{
		UserId: uid,
		Title:  "新对话",
	})
}

func (s *chatService) ListConversations(ctx context.Context, uid int64) ([]domain.Conversation, error) {
	return s.convRepo.List(ctx, uid)
}

func (s *chatService) DeleteConversation(ctx context.Context, uid int64, convId int64) error {
	return s.convRepo.Delete(ctx, uid, convId)
}

func (s *chatService) ListMessages(ctx context.Context, uid int64, convId int64, beforeId int64, limit int) ([]domain.Message, error) {
	_, err := s.convRepo.Find(ctx, uid, convId)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if beforeId > 0 {
		return s.msgRepo.ListBefore(ctx, convId, beforeId, limit)
	}
	return s.msgRepo.ListRecent(ctx, convId, limit)
}

func (s *chatService) SendMessage(ctx context.Context, uid int64, convId int64, content string) (<-chan domain.ChatEvent, error) {
	// 1. 校验对话归属
	_, err := s.convRepo.Find(ctx, uid, convId)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("查询对话失败: %w", err)
	}

	// 2. 校验消息长度
	if utf8.RuneCountInString(content) > maxMessageLength {
		return nil, ErrMessageTooLong
	}

	// 3. 保存用户消息
	_, err = s.msgRepo.Insert(ctx, domain.Message{
		ConversationId: convId,
		Role:           "user",
		Content:        content,
	})
	if err != nil {
		return nil, fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 4. 构建 prompt
	messages, err := s.buildPrompt(ctx, convId)
	if err != nil {
		return nil, fmt.Errorf("构建 prompt 失败: %w", err)
	}

	// 4.1 RAG：检索相关文章，注入上下文
	if articles := s.searchArticles(ctx, content); len(articles) > 0 {
		messages = s.injectArticleContext(messages, articles)
	}

	// 5. 创建可取消的 context
	streamCtx, cancel := context.WithCancel(ctx)
	s.cancel.Store(cancelKey(uid, convId), cancel)

	// 6. 首次调用 LLM（同步检测连接错误）
	var tools []ai.Tool
	if s.executor != nil {
		tools = s.executor.Definitions()
	}
	llmCh, err := s.llm.ChatStream(streamCtx, messages, tools)
	if err != nil {
		cancel()
		s.cancel.Delete(cancelKey(uid, convId))
		return nil, fmt.Errorf("调用 LLM 失败: %w", err)
	}

	// 7. goroutine 处理流（含工具调用循环）
	eventCh := make(chan domain.ChatEvent, 16)
	go s.runStream(streamCtx, convId, uid, messages, llmCh, eventCh)

	return eventCh, nil
}

func (s *chatService) StopGeneration(ctx context.Context, uid int64, convId int64) error {
	if cancelFn, ok := s.cancel.LoadAndDelete(cancelKey(uid, convId)); ok {
		cancelFn.(context.CancelFunc)()
	}
	return nil
}

// cancelKey 生成 uid:convId 复合 key，防止越权取消他人的生成
func cancelKey(uid, convId int64) string {
	return fmt.Sprintf("%d:%d", uid, convId)
}

// buildPrompt 构建系统提示词 + 最近历史
// 只取最近 maxHistoryRounds*2 条，不全量加载
func (s *chatService) buildPrompt(ctx context.Context, convId int64) ([]ai.ChatMessage, error) {
	recentMsgs, err := s.msgRepo.ListRecent(ctx, convId, maxHistoryRounds*2)
	if err != nil {
		return nil, err
	}

	messages := make([]ai.ChatMessage, 0, len(recentMsgs)+1)
	messages = append(messages, ai.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})
	for _, m := range recentMsgs {
		messages = append(messages, ai.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return messages, nil
}

// searchArticles 检索相关文章，失败静默降级
func (s *chatService) searchArticles(ctx context.Context, query string) []domain.Article {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	articles, _, err := s.search.Search(ctx, query, 1, ragTopK)
	if err != nil {
		s.l.Warn("RAG 检索失败，降级为无 RAG",
			logger.String("query", query),
			logger.Error(err))
		return nil
	}
	return articles
}

// injectArticleContext 将检索到的文章注入 prompt，插入到 system prompt 之后
func (s *chatService) injectArticleContext(messages []ai.ChatMessage, articles []domain.Article) []ai.ChatMessage {
	var buf strings.Builder
	buf.WriteString("以下是平台相关文章，请基于这些内容回答用户问题。引用时直接使用提供的 Markdown 链接。\n\n")
	for i, a := range articles {
		fmt.Fprintf(&buf, "[%d] [%s](/article/%d) — %s（作者：%s）\n\n",
			i+1, a.Title, a.Id, a.Abstract, a.Author.Name)
	}

	result := make([]ai.ChatMessage, 0, len(messages)+1)
	result = append(result, messages[0]) // 原 system prompt
	result = append(result, ai.ChatMessage{Role: "system", Content: buf.String()})
	result = append(result, messages[1:]...) // 历史消息
	return result
}

// runStream 处理 LLM 流，支持工具调用循环，最多 maxToolRounds 轮
// messages 用于工具调用后追加历史并发起第二轮 LLM 调用
func (s *chatService) runStream(
	ctx context.Context,
	convId, uid int64,
	messages []ai.ChatMessage,
	llmCh <-chan ai.StreamChunk,
	eventCh chan<- domain.ChatEvent,
) {
	defer close(eventCh)
	defer s.cancel.Delete(cancelKey(uid, convId))

	var fullContent strings.Builder

	for round := 0; round <= maxToolRounds; round++ {
		toolCalls, usage, finished := s.processChunks(ctx, convId, uid, llmCh, &fullContent, eventCh)
		if !finished {
			return
		}

		if len(toolCalls) == 0 {
			reply := fullContent.String()
			msgId := s.saveReply(convId, uid, reply)
			s.trySend(ctx, eventCh, domain.ChatEvent{
				Type: "done",
				Data: map[string]any{
					"messageId": msgId,
					"usage":     buildUsage(usage),
				},
			})
			return
		}

		if round == maxToolRounds {
			s.l.Warn("工具调用超过最大轮次", logger.Int64("convId", convId))
			reply := fullContent.String()
			msgId := s.saveReply(convId, uid, reply)
			s.trySend(ctx, eventCh, domain.ChatEvent{
				Type: "done",
				Data: map[string]any{"messageId": msgId, "usage": buildUsage(usage)},
			})
			return
		}

		// 执行工具，把 assistant tool_call + tool result 追加到 messages
		messages = s.executeTools(ctx, uid, messages, toolCalls, eventCh)

		// 发起下一轮 LLM 调用
		var tools []ai.Tool
		if s.executor != nil {
			tools = s.executor.Definitions()
		}
		var err error
		llmCh, err = s.llm.ChatStream(ctx, messages, tools)
		if err != nil {
			s.trySend(ctx, eventCh, domain.ChatEvent{Type: "error", Content: "工具调用后 LLM 请求失败"})
			s.savePartialReply(convId, uid, fullContent.String())
			return
		}
	}
}

// processChunks 处理单轮 LLM stream
// finished=true 表示收到 done 或 tool_call，false 表示 ctx 取消/异常
func (s *chatService) processChunks(
	ctx context.Context,
	convId, uid int64,
	llmCh <-chan ai.StreamChunk,
	fullContent *strings.Builder,
	eventCh chan<- domain.ChatEvent,
) (toolCalls []ai.StreamToolCall, usage *ai.StreamUsage, finished bool) {
	for {
		select {
		case <-ctx.Done():
			s.savePartialReply(convId, uid, fullContent.String())
			return nil, nil, false
		case chunk, ok := <-llmCh:
			if !ok {
				s.savePartialReply(convId, uid, fullContent.String())
				return nil, nil, false
			}
			switch chunk.Type {
			case "text":
				fullContent.WriteString(chunk.Content)
				if !s.trySend(ctx, eventCh, domain.ChatEvent{Type: "delta", Content: chunk.Content}) {
					s.savePartialReply(convId, uid, fullContent.String())
					return nil, nil, false
				}
			case "tool_call":
				return chunk.ToolCalls, chunk.Usage, true
			case "error":
				s.trySend(ctx, eventCh, domain.ChatEvent{Type: "error", Content: chunk.Content})
				s.savePartialReply(convId, uid, fullContent.String())
				return nil, nil, false
			case "done":
				return nil, chunk.Usage, true
			}
		}
	}
}

// executeTools 执行工具调用列表，发送 tool_call + tool_result 事件，并把结果追加到 messages
func (s *chatService) executeTools(
	ctx context.Context,
	uid int64,
	messages []ai.ChatMessage,
	toolCalls []ai.StreamToolCall,
	eventCh chan<- domain.ChatEvent,
) []ai.ChatMessage {
	// 组装 assistant 消息（携带 tool_calls）
	assistantMsg := ai.ChatMessage{Role: "assistant"}
	for _, tc := range toolCalls {
		assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, ai.ToolCallData{
			Id:   tc.Id,
			Type: "function",
			Function: ai.ToolCallFunction{
				Name:      tc.Name,
				Arguments: marshalArgs(tc.Args),
			},
		})
	}
	messages = append(messages, assistantMsg)

	for _, tc := range toolCalls {
		// 通知前端工具调用开始
		s.trySend(ctx, eventCh, domain.ChatEvent{
			Type: "tool_call",
			Data: map[string]any{"id": tc.Id, "name": tc.Name, "args": tc.Args},
		})

		var result domain.ToolResultData
		if s.executor != nil {
			var err error
			result, err = s.executor.Execute(ctx, uid, tc.Name, tc.Args)
			if err != nil {
				result = domain.ToolResultData{Name: tc.Name, Error: err.Error()}
			}
		} else {
			result = domain.ToolResultData{Name: tc.Name, Error: "no executor"}
		}
		result.CallId = tc.Id

		// 发送 tool_result 事件给前端
		s.trySend(ctx, eventCh, domain.ChatEvent{Type: "tool_result", Data: result})

		// 把工具结果回注到 messages（tool role）
		resultContent := marshalResult(result)
		messages = append(messages, ai.ChatMessage{
			Role:       "tool",
			ToolCallId: tc.Id,
			Content:    resultContent,
		})
	}
	return messages
}

// marshalArgs 将 args map 序列化为 JSON 字符串
func marshalArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// marshalResult 将工具结果序列化为传给 LLM 的 JSON 字符串
func marshalResult(r domain.ToolResultData) string {
	if r.Error != "" {
		return fmt.Sprintf(`{"error":%q}`, r.Error)
	}
	b, err := json.Marshal(r)
	if err != nil {
		return `{"error":"序列化失败"}`
	}
	return string(b)
}

// trySend 尝试发送事件，如果 ctx 已取消则放弃（防止 goroutine 阻塞泄漏）
func (s *chatService) trySend(ctx context.Context, ch chan<- domain.ChatEvent, event domain.ChatEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// saveReply 保存完整 AI 回复，返回消息 ID
func (s *chatService) saveReply(convId int64, uid int64, reply string) int64 {
	if reply == "" {
		return 0
	}
	saved, err := s.msgRepo.Insert(context.Background(), domain.Message{
		ConversationId: convId,
		Role:           "assistant",
		Content:        reply,
	})
	if err != nil {
		s.l.Error("保存 AI 回复失败",
			logger.Int64("convId", convId),
			logger.Error(err))
		return 0
	}
	s.autoTitle(convId, uid, reply)
	return saved.Id
}

// savePartialReply 前端断开或出错时保存已有的部分回复
func (s *chatService) savePartialReply(convId int64, uid int64, content string) {
	if content == "" {
		return
	}
	s.saveReply(convId, uid, content)
}

// autoTitle 首条对话时自动截取 AI 回复前 N 字作为标题
func (s *chatService) autoTitle(convId int64, uid int64, reply string) {
	// 只取前 3 条判断是否首轮对话，不全量加载
	msgs, err := s.msgRepo.ListRecent(context.Background(), convId, 3)
	if err != nil || len(msgs) > 2 {
		return
	}

	title := truncateRunes(reply, titleMaxRunes)
	if title == "" {
		return
	}
	if err := s.convRepo.UpdateTitle(context.Background(), uid, convId, title); err != nil {
		s.l.Error("自动生成对话标题失败",
			logger.Int64("convId", convId),
			logger.Error(err))
	}
}

// isNotFound 判断是否"记录不存在"错误（通过 repository 层错误链，不直接依赖 GORM）
func isNotFound(err error) bool {
	return errors.Is(err, repository.ErrRecordNotFound)
}

// buildUsage 将 StreamUsage 转为 done 事件的 usage map
func buildUsage(u *ai.StreamUsage) map[string]int {
	if u == nil {
		return map[string]int{"promptTokens": 0, "completionTokens": 0}
	}
	return map[string]int{
		"promptTokens":     u.PromptTokens,
		"completionTokens": u.CompletionTokens,
	}
}

// truncateRunes 截取前 n 个 rune，去除换行
func truncateRunes(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
