package setup

import (
	"context"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
)

// FakeInteractionService 集成测试桩：互动已拆为 webook-interaction 独立服务，
// core 集成测试不拉起 interaction gRPC server，article 用例也不校验互动数据，故全返零值/空。
// 真要端到端测互动走 webook/interaction/integration（bufconn 真 gRPC）。
type FakeInteractionService struct{}

func newFakeInteractionService() service.InteractionService {
	return FakeInteractionService{}
}

func (FakeInteractionService) IncrReadCount(context.Context, string, int64) error        { return nil }
func (FakeInteractionService) Like(context.Context, int64, string, int64) error          { return nil }
func (FakeInteractionService) CancelLike(context.Context, int64, string, int64) error    { return nil }
func (FakeInteractionService) Collect(context.Context, int64, string, int64) error       { return nil }
func (FakeInteractionService) CancelCollect(context.Context, int64, string, int64) error { return nil }

func (FakeInteractionService) FindInteraction(context.Context, int64, string, int64) (domain.Interaction, error) {
	return domain.Interaction{}, nil
}

func (FakeInteractionService) FindUserState(context.Context, int64, string, int64) (bool, bool, error) {
	return false, false, nil
}

func (FakeInteractionService) FindByBizIds(context.Context, string, []int64) (map[int64]domain.Interaction, error) {
	return map[int64]domain.Interaction{}, nil
}

func (FakeInteractionService) FindUserLiked(context.Context, int64, string, []int64) (map[int64]bool, error) {
	return map[int64]bool{}, nil
}

func (FakeInteractionService) ListHotBizIds(context.Context, string, int) ([]int64, error) {
	return nil, nil
}

func (FakeInteractionService) ListCollectedBizIds(context.Context, int64, string, int) ([]int64, error) {
	return nil, nil
}
