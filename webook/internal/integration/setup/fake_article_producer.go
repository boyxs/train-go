package setup

import (
	"context"

	articleevt "github.com/boyxs/train-go/webook/internal/events/article"
)

// fakeArticleEventProducer：集成测试不拉 kafka，article 事件生产 no-op（对齐 relation 侧 nil producer 约定，
// 但用显式 no-op 桩避免 Publish/Withdraw 路径 nil 解引用 panic）。
type fakeArticleEventProducer struct{}

func NewFakeArticleEventProducer() articleevt.ArticleEventProducer {
	return &fakeArticleEventProducer{}
}

func (f *fakeArticleEventProducer) Produce(context.Context, articleevt.ArticleEvent) error {
	return nil
}
