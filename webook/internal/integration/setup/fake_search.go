package setup

import (
	"context"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/service"
)

// FakeSearchService / FakeTagService 集成测试桩：search/tag 已拆为独立 gRPC 服务，
// core 集成测试不拉起它们（也不校验搜索/标签数据），发布链路后台的索引/同步调用全空转、返零值。
// 端到端测搜索/标签走 webook/search、webook/tag 各自 integration。

type FakeSearchService struct{}

func newFakeSearchService() service.ArticleSearchService { return FakeSearchService{} }

func (FakeSearchService) Index(context.Context, domain.Article) error { return nil }
func (FakeSearchService) Remove(context.Context, int64) error         { return nil }

func (FakeSearchService) Search(context.Context, string, []string, int, int) (domain.SearchResult, error) {
	return domain.SearchResult{}, nil
}

type FakeTagService struct{}

func newFakeTagService() service.TagService { return FakeTagService{} }

func (FakeTagService) SyncTags(context.Context, string, int64, []string, string) ([]domain.Tag, error) {
	return nil, nil
}
func (FakeTagService) ClearTags(context.Context, string, int64) error { return nil }
func (FakeTagService) Suggest(context.Context, string, int) ([]domain.Tag, error) {
	return nil, nil
}
func (FakeTagService) Recommend(context.Context, string, string) ([]domain.TagCount, error) {
	return nil, nil
}
func (FakeTagService) Detail(context.Context, string, int64) (domain.Tag, bool, error) {
	return domain.Tag{}, false, nil
}
func (FakeTagService) Follow(context.Context, int64, string) (bool, int64, error) {
	return false, 0, nil
}
func (FakeTagService) Unfollow(context.Context, int64, string) (bool, int64, error) {
	return false, 0, nil
}
func (FakeTagService) TagArticles(context.Context, string, string, int, int) (domain.SearchResult, error) {
	return domain.SearchResult{}, nil
}
func (FakeTagService) TagsByBiz(context.Context, string, []int64) (map[int64][]domain.Tag, error) {
	return map[int64][]domain.Tag{}, nil
}
