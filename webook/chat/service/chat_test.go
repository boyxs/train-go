package service

import (
	"context"
	"testing"

	"github.com/boyxs/train-go/webook/chat/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// stubMsgRepo 提示词构建测试
type stubMsgRepo struct {
	recent     []domain.Message
	deletedIds []int64
	updatedIds []int64
}

func (s *stubMsgRepo) Insert(context.Context, domain.Message) (domain.Message, error) {
	return domain.Message{}, nil
}
func (s *stubMsgRepo) UpdateContent(_ context.Context, _ int64, id int64, _ string, _ string) error {
	s.updatedIds = append(s.updatedIds, id)
	return nil
}
func (s *stubMsgRepo) Delete(_ context.Context, _ int64, id int64) error {
	s.deletedIds = append(s.deletedIds, id)
	return nil
}
func (s *stubMsgRepo) UpdateFeedback(context.Context, int64, int64, int8) error { return nil }
func (s *stubMsgRepo) ListRecent(context.Context, int64, int) ([]domain.Message, error) {
	return s.recent, nil
}
func (s *stubMsgRepo) ListRecentLite(context.Context, int64, int) ([]domain.Message, error) {
	return s.recent, nil
}
func (s *stubMsgRepo) ListBefore(context.Context, int64, int64, int) ([]domain.Message, error) {
	return s.recent, nil
}
func (s *stubMsgRepo) ListAll(context.Context, int64) ([]domain.Message, error) {
	return s.recent, nil
}

// 空 assistant 占位消息（生成被中断前内容为空残留 DB）不得进入 LLM prompt，
// 否则 DeepSeek 报 400 "assistant message must have content or tool_calls"。
func TestBuildPrompt_SkipsEmptyAssistant(t *testing.T) {
	repo := &stubMsgRepo{recent: []domain.Message{
		{Role: "user", Content: "1"},
		{Role: "assistant", Content: ""}, // 残留空占位（连发被中断）
		{Role: "user", Content: "0"},
	}}
	s := &AIChatService{msgRepo: repo, l: logger.NewNopLogger()}

	messages, err := s.buildPrompt(context.Background(), 1)
	if err != nil {
		t.Fatalf("buildPrompt err: %v", err)
	}

	for i, m := range messages {
		if m.Role == "assistant" && m.Content == "" {
			t.Errorf("messages[%d] 是空 assistant，会触发 DeepSeek 400", i)
		}
	}

	// 期望：system + user("1") + user("0") = 3 条（空 assistant 已过滤）
	if len(messages) != 3 {
		t.Errorf("messages 数量 = %d, want 3 (system + 2 user)", len(messages))
	}
}

// 正常 assistant（有内容）必须保留。
func TestBuildPrompt_KeepsNonEmptyAssistant(t *testing.T) {
	repo := &stubMsgRepo{recent: []domain.Message{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好，有什么可以帮你"},
	}}
	s := &AIChatService{msgRepo: repo, l: logger.NewNopLogger()}

	messages, err := s.buildPrompt(context.Background(), 1)
	if err != nil {
		t.Fatalf("buildPrompt err: %v", err)
	}
	// system + user + assistant = 3
	if len(messages) != 3 {
		t.Errorf("messages 数量 = %d, want 3", len(messages))
	}
}

// 中断时空内容必须删除占位行（savePartialReply），非空则更新。
func TestSavePartialReply_EmptyDeletesPlaceholder(t *testing.T) {
	repo := &stubMsgRepo{}
	s := &AIChatService{msgRepo: repo, l: logger.NewNopLogger()}

	s.savePartialReply(42, 1, "") // 空内容 → 删占位
	if len(repo.deletedIds) != 1 || repo.deletedIds[0] != 42 {
		t.Errorf("空内容应删除占位 42，got deleted=%v", repo.deletedIds)
	}
	if len(repo.updatedIds) != 0 {
		t.Errorf("空内容不应 UpdateContent，got updated=%v", repo.updatedIds)
	}

	repo2 := &stubMsgRepo{}
	s2 := &AIChatService{msgRepo: repo2, l: logger.NewNopLogger()}
	s2.savePartialReply(43, 1, "部分内容") // 非空 → 更新不删
	if len(repo2.deletedIds) != 0 {
		t.Errorf("非空内容不应删除，got deleted=%v", repo2.deletedIds)
	}
	if len(repo2.updatedIds) != 1 || repo2.updatedIds[0] != 43 {
		t.Errorf("非空内容应 UpdateContent 43，got updated=%v", repo2.updatedIds)
	}
}
