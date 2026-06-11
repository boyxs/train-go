package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/errs"
	"github.com/webook/migrator/repository"
	"github.com/webook/pkg/logger"
)

// CreateReq 创建任务的 service 层入参。
// Handler 层负责把 HTTP JSON 转成这个 struct（解耦 HTTP 细节）。
type CreateReq struct {
	Name         string
	Mode         domain.Mode
	Kind         domain.Kind
	SourceType   domain.SourceType
	SourceDsnRef string
	SinkType     string
	SinkDsnRef   string
	Tables       []domain.TableMapping
}

type TaskService interface {
	Create(ctx context.Context, req CreateReq) (int64, error)
	Get(ctx context.Context, id int64) (domain.Task, error)
	List(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error)
	// UpdateStatus 推进 task.status — 引擎层（Full/Incr）在 Run 入口/出口、
	// SwitchService 进 DST_ONLY 时调，将运行状态落库供 List/Get 暴露给运维。
	UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus) error
	// SetThrottle 写 task 级限速配置（"下次 Start 生效"语义）；存储未装配 → ErrThrottleNotConfigured。
	SetThrottle(ctx context.Context, id int64, cfg domain.ThrottleConfig) error
	// ClearThrottle 清空限速配置（恢复默认）；存储未装配 → ErrThrottleNotConfigured。
	ClearThrottle(ctx context.Context, id int64) error
	// GetThrottle 读限速配置；未设置或存储未装配 → (zero, false, nil)。
	GetThrottle(ctx context.Context, id int64) (domain.ThrottleConfig, bool, error)
}

type InternalTaskService struct {
	repo         repository.TaskRepository
	throttleRepo repository.ThrottleRepository // 可空（wire 未注入时 throttle 端点返 501）
	l            logger.LoggerX
}

func NewTaskService(r repository.TaskRepository, throttleRepo repository.ThrottleRepository, l logger.LoggerX) TaskService {
	return &InternalTaskService{repo: r, throttleRepo: throttleRepo, l: l}
}

func (s *InternalTaskService) Create(ctx context.Context, req CreateReq) (int64, error) {
	if err := s.validate(req); err != nil {
		return 0, err // 已是 *errs.Error（含 cause），handler 透传即可
	}
	tablesJSON, err := json.Marshal(req.Tables)
	if err != nil {
		return 0, errs.ErrInvalidArgument.WithCause(err)
	}
	t := domain.Task{
		Name:         req.Name,
		Mode:         req.Mode,
		Kind:         req.Kind,
		SourceType:   req.SourceType.Normalize(),
		SourceDsnRef: req.SourceDsnRef,
		SinkType:     req.SinkType,
		SinkDsnRef:   req.SinkDsnRef,
		TablesJSON:   string(tablesJSON),
		Status:       domain.TaskStatusCreated,
		Consistency:  "eventual",
	}
	return s.repo.Create(ctx, t)
}

func (s *InternalTaskService) Get(ctx context.Context, id int64) (domain.Task, error) {
	return s.repo.FindById(ctx, id)
}

func (s *InternalTaskService) List(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error) {
	return s.repo.List(ctx, opts)
}

func (s *InternalTaskService) UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *InternalTaskService) SetThrottle(ctx context.Context, id int64, cfg domain.ThrottleConfig) error {
	if s.throttleRepo == nil {
		return errs.ErrThrottleNotConfigured
	}
	return s.throttleRepo.Save(ctx, id, cfg)
}

func (s *InternalTaskService) ClearThrottle(ctx context.Context, id int64) error {
	if s.throttleRepo == nil {
		return errs.ErrThrottleNotConfigured
	}
	return s.throttleRepo.Clear(ctx, id)
}

func (s *InternalTaskService) GetThrottle(ctx context.Context, id int64) (domain.ThrottleConfig, bool, error) {
	if s.throttleRepo == nil {
		// Start 回读路径的软依赖：未装配等价"未设置"，用引擎默认，不报错
		return domain.ThrottleConfig{}, false, nil
	}
	return s.throttleRepo.Find(ctx, id)
}

func (s *InternalTaskService) validate(r CreateReq) error {
	switch {
	case r.Name == "":
		return errs.ErrInvalidArgument.WithCause(errors.New("name 不能为空"))
	case !r.Mode.Valid():
		return errs.ErrInvalidArgument.WithCause(fmt.Errorf("mode 不合法: %q", r.Mode))
	case !r.Kind.Valid():
		return errs.ErrInvalidArgument.WithCause(fmt.Errorf("kind 不合法: %q", r.Kind))
	case !r.SourceType.Normalize().Valid():
		return errs.ErrInvalidArgument.WithCause(fmt.Errorf("sourceType 不合法: %q", r.SourceType))
	case r.SourceDsnRef == "":
		return errs.ErrInvalidArgument.WithCause(errors.New("sourceDsnRef 不能为空"))
	case r.SinkType == "":
		return errs.ErrInvalidArgument.WithCause(errors.New("sinkType 不能为空"))
	case r.SinkDsnRef == "":
		return errs.ErrInvalidArgument.WithCause(errors.New("sinkDsnRef 不能为空"))
	case len(r.Tables) == 0:
		return errs.ErrInvalidArgument.WithCause(errors.New("tables 至少 1 张"))
	}
	return nil
}
