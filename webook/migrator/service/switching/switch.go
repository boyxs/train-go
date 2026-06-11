// Package switching 是切流引擎。
//
// 设计（architecture.md §5 + §8.3 + §14 任务 #6）：
//
//	SetGray   设置灰度比例 0-100；持久化到 Redis + 同步 task.gray_percent
//	SetStage  切流阶段推进；进 DST_ONLY 需要双人复核（propose/approve 不同 actor）
//	Rollback  双写期（SRC_FIRST/DST_FIRST）回 SRC_FIRST（幂等）；DST_ONLY/SRC_ONLY 拒绝
//	GetStage  读当前 stage（默认 SRC_ONLY）
//	GetGray   读当前灰度
//
// 持久化经 repository.SwitchStateRepository（Redis 键定义见 repository/cache/switch_state.go，
// key 按 task.Name 而非 taskId，跟 SDK [`internal/migratorsdk/redis.go`] 对齐 —— 业务方只
// 从 yaml `migrator.sdk.taskName` 拿到 name，不知道 ID）：
//
//	migrator:stage:{taskName}            → string Stage
//	migrator:gray:{taskName}             → int percent
//	migrator:cutover_propose:{taskName}  → propose actor_id（TTL 10 分钟，过期需重新 propose）
package switching

import (
	"context"
	"fmt"

	"github.com/webook/migrator/domain"
	migratorerrs "github.com/webook/migrator/errs"
	"github.com/webook/migrator/repository"
	"github.com/webook/pkg/errs"
	"github.com/webook/pkg/logger"
)

// ErrApprovalSameActor 双人复核的 propose 与 approve 是同一 actor。
// HTTP 409 — 状态冲突（流程要求两个不同 actor）。
var ErrApprovalSameActor = errs.New(409, "propose 和 approve 必须是不同用户")

// ErrProposeNotFound approve 时没有找到 active 的 propose（未提议或已过期）。
// HTTP 412 — 前置条件未满足（需先 propose 再 approve）。
var ErrProposeNotFound = errs.New(412, "未提议或提议已过期，请先 propose")

// SwitchService 切流引擎接口。
type SwitchService interface {
	SetGray(ctx context.Context, taskId int64, percent int) error
	GetGray(ctx context.Context, taskId int64) (int, error)
	// SetStage 切流阶段推进。进 DST_ONLY 需要 propose+approve 都填且不同。其它阶段两个 actor 字段忽略。
	SetStage(ctx context.Context, taskId int64, next domain.Stage, propose, approve string) error
	GetStage(ctx context.Context, taskId int64) (domain.Stage, error)
	// Rollback 双写期（SRC_FIRST/DST_FIRST）回 SRC_FIRST（幂等：已在 SRC_FIRST 则 no-op）；
	// DST_ONLY 单写不可逆、SRC_ONLY 未进双写均拒绝（返 ErrRollbackNotAllowed）。
	Rollback(ctx context.Context, taskId int64) error
}

type InternalSwitchService struct {
	repo  repository.TaskRepository
	state repository.SwitchStateRepository
	l     logger.LoggerX
}

func NewSwitchService(repo repository.TaskRepository, state repository.SwitchStateRepository, l logger.LoggerX) SwitchService {
	return &InternalSwitchService{repo: repo, state: state, l: l}
}

func (s *InternalSwitchService) SetGray(ctx context.Context, taskId int64, percent int) error {
	if percent < 0 || percent > 100 {
		return migratorerrs.ErrInvalidGrayPercent
	}
	task, err := s.repo.FindById(ctx, taskId)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	if err := s.state.SetGray(ctx, task.Name, percent); err != nil {
		return fmt.Errorf("set gray redis: %w", err)
	}
	if err := s.repo.UpdateGrayPercent(ctx, taskId, int16(percent)); err != nil {
		s.l.Warn("sync gray to task table failed",
			logger.Int64("task_id", taskId),
			logger.Int("percent", percent),
			logger.Error(err))
		// task table 是冗余持久化，Redis 才是路由决策源；这里不阻塞
	}
	return nil
}

func (s *InternalSwitchService) GetGray(ctx context.Context, taskId int64) (int, error) {
	task, err := s.repo.FindById(ctx, taskId)
	if err != nil {
		return 0, fmt.Errorf("find task: %w", err)
	}
	return s.state.GetGray(ctx, task.Name)
}

func (s *InternalSwitchService) SetStage(ctx context.Context, taskId int64, next domain.Stage, propose, approve string) error {
	if !next.Valid() {
		return fmt.Errorf("invalid stage %q", next)
	}
	task, err := s.repo.FindById(ctx, taskId)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}

	cur, err := s.GetStage(ctx, taskId)
	if err != nil {
		return fmt.Errorf("get current stage: %w", err)
	}
	if cur == next {
		// 幂等：同 stage 重复 SetStage 不报错（API 重试场景）
		return nil
	}
	if !cur.CanTransitionTo(next) {
		// 动态 from/to 通过 metadata 返给前端（响应 body 里的 metadata 字段）；
		// sentinel.Message 保持固定 "状态机非法转移" 供 errors.Is 比对；
		// 详细 cause 进 log + audit_log.error_msg。
		return migratorerrs.ErrInvalidStatusTransition.
			WithCause(fmt.Errorf("%s → %s 非法（必须按 SRC_ONLY→SRC_FIRST→DST_FIRST→DST_ONLY 顺序）", cur, next)).
			WithMetadata("from", string(cur)).
			WithMetadata("to", string(next)).
			WithMetadata("allowed", "SRC_ONLY→SRC_FIRST→DST_FIRST→DST_ONLY")
	}

	// 进 DST_ONLY 需要双人复核
	if next == domain.StageDstOnly {
		if err := s.consumePropose(ctx, task.Name, propose, approve); err != nil {
			return err
		}
	}

	if err := s.state.SetStage(ctx, task.Name, next); err != nil {
		return fmt.Errorf("set stage redis: %w", err)
	}
	// 进 DST_ONLY → task.status 同步推进到 switched（运维 List/Get 能看到）。
	// 其他 stage 不动 status：SRC_FIRST/DST_FIRST 期间 status 由引擎维护（保持 incr_running）。
	if next == domain.StageDstOnly {
		if err := s.repo.UpdateStatus(ctx, taskId, domain.TaskStatusSwitched); err != nil {
			s.l.Warn("sync task.status=switched failed",
				logger.Int64("task_id", taskId), logger.Error(err))
		}
	}
	s.l.Info("stage transitioned",
		logger.Int64("task_id", taskId),
		logger.String("from", string(cur)),
		logger.String("to", string(next)))
	return nil
}

// consumePropose 双人复核流程：
//   - propose != "" && approve == ""  → 注册 propose 并立即返回 ErrProposeNotFound（标记"已 propose，等 approve"）
//   - propose != "" && approve != ""  → 验证已 propose 的 actor + approve 不同 → 通过
//   - propose == "" && approve != ""  → 兜底读 Redis 拿 active propose，验证 approve 与之不同 → 通过
//
// API 设计上鼓励"两次调用"——第一次只传 propose 触发"已提议"状态，第二次只传 approve 验证。
// 本方法也兼容"propose + approve 同次提供"（用于测试 / 自动化场景）。
func (s *InternalSwitchService) consumePropose(ctx context.Context, taskName string, propose, approve string) error {
	if approve == "" {
		// 只 propose，没 approve → 注册 propose 后报 ErrProposeNotFound 通知"还需要第二人 approve"
		if propose == "" {
			return migratorerrs.ErrProposeActorRequired
		}
		if err := s.state.SavePropose(ctx, taskName, propose); err != nil {
			return fmt.Errorf("save propose: %w", err)
		}
		return fmt.Errorf("propose registered for actor %q; second approver must call again with approve param: %w", propose, ErrProposeNotFound)
	}

	// approve != "" → 必须能拿到 propose actor
	if propose == "" {
		got, err := s.state.FindPropose(ctx, taskName)
		if err != nil {
			return fmt.Errorf("read propose: %w", err)
		}
		if got == "" {
			return ErrProposeNotFound
		}
		propose = got
	}
	if propose == approve {
		return ErrApprovalSameActor
	}
	// 双人通过 → 删除 propose key（防 replay）
	if err := s.state.DeletePropose(ctx, taskName); err != nil {
		s.l.Warn("delete propose key failed", logger.String("task_name", taskName), logger.Error(err))
	}
	return nil
}

func (s *InternalSwitchService) GetStage(ctx context.Context, taskId int64) (domain.Stage, error) {
	task, err := s.repo.FindById(ctx, taskId)
	if err != nil {
		return "", fmt.Errorf("find task: %w", err)
	}
	v, err := s.state.GetStage(ctx, task.Name)
	if err != nil {
		return "", err
	}
	if v == "" {
		// 未设置 → 默认 SRC_ONLY（未开始切流）
		return domain.StageSrcOnly, nil
	}
	return v, nil
}

func (s *InternalSwitchService) Rollback(ctx context.Context, taskId int64) error {
	task, err := s.repo.FindById(ctx, taskId)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	cur, err := s.GetStage(ctx, taskId)
	if err != nil {
		return err
	}
	if !cur.CanRollback() {
		// SRC_ONLY 未进双写 / DST_ONLY 单写不可逆（切回会读到停更的脏 OLD）→ 拒绝
		s.l.Warn("rollback rejected: stage not in dual-write phase",
			logger.Int64("task_id", taskId), logger.String("stage", string(cur)))
		return migratorerrs.ErrRollbackNotAllowed.
			WithCause(fmt.Errorf("当前 stage=%s，仅双写期 SRC_FIRST/DST_FIRST 可回滚", cur)).
			WithMetadata("from", string(cur)).
			WithMetadata("allowed", "SRC_FIRST/DST_FIRST")
	}
	// 幂等：已在 SRC_FIRST 不做事
	if cur == domain.StageSrcFirst {
		return nil
	}
	if err := s.state.SetStage(ctx, task.Name, domain.StageSrcFirst); err != nil {
		return fmt.Errorf("rollback stage: %w", err)
	}
	s.l.Info("rollback to SRC_FIRST",
		logger.Int64("task_id", taskId),
		logger.String("task_name", task.Name),
		logger.String("from", string(cur)))
	return nil
}
