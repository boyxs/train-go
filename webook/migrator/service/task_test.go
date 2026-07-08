package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/errs"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// newSvc 测试 helper：注入 NopLogger，每个 case 复用。
func newSvc(repo repository.TaskRepository) TaskService {
	return NewTaskService(repo, nil, logger.NewNopLogger())
}

// ── 手写 stub Repository ───────────────────────────────────
type stubTaskRepository struct {
	CreateFn   func(ctx context.Context, t domain.Task) (int64, error)
	FindByIdFn func(ctx context.Context, id int64) (domain.Task, error)
	ListFn     func(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error)
}

func (m *stubTaskRepository) Create(ctx context.Context, t domain.Task) (int64, error) {
	return m.CreateFn(ctx, t)
}
func (m *stubTaskRepository) FindById(ctx context.Context, id int64) (domain.Task, error) {
	return m.FindByIdFn(ctx, id)
}
func (m *stubTaskRepository) List(ctx context.Context, opts repository.ListOpts) ([]domain.Task, int64, error) {
	return m.ListFn(ctx, opts)
}
func (m *stubTaskRepository) UpdateStatus(_ context.Context, _ int64, _ domain.TaskStatus) error {
	return nil
}
func (m *stubTaskRepository) UpdateGrayPercent(_ context.Context, _ int64, _ int16) error {
	return nil
}

// ── 测试 ───────────────────────────────────────────────────
func validReq() CreateReq {
	return CreateReq{
		Name: "article_to_es_v1", Mode: domain.ModeCDC, Kind: domain.KindHeterogeneous,
		SourceDsnRef: "vault:src", SinkType: "es", SinkDsnRef: "vault:dst",
		Tables: []domain.TableMapping{{Src: "article", Dst: "article_v1", PartitionKey: "id"}},
	}
}

func TestTaskService_Create_Validate(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*CreateReq)
		want   string // 期望的错误 message 子串（在 ErrInvalidArgument wrap 后）
	}{
		{"name 空", func(r *CreateReq) { r.Name = "" }, "name 不能为空"},
		{"mode 不合法", func(r *CreateReq) { r.Mode = "weird" }, "mode 不合法"},
		{"kind 不合法", func(r *CreateReq) { r.Kind = "weird" }, "kind 不合法"},
		{"sourceDsnRef 空", func(r *CreateReq) { r.SourceDsnRef = "" }, "sourceDsnRef 不能为空"},
		{"sinkType 空", func(r *CreateReq) { r.SinkType = "" }, "sinkType 不能为空"},
		{"sinkDsnRef 空", func(r *CreateReq) { r.SinkDsnRef = "" }, "sinkDsnRef 不能为空"},
		{"tables 空", func(r *CreateReq) { r.Tables = nil }, "tables 至少 1 张"},
		{"sourceType 不合法", func(r *CreateReq) { r.SourceType = "redis" }, "sourceType 不合法"},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			req := validReq()
			c.mutate(&req)
			svc := newSvc(&stubTaskRepository{
				CreateFn: func(ctx context.Context, _ domain.Task) (int64, error) {
					t.Fatal("repo.Create should not be called when validation fails")
					return 0, nil
				},
			})
			_, err := svc.Create(context.Background(), req)
			assert.ErrorIs(t, err, errs.ErrInvalidArgument)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

func TestTaskService_Create_Success(t *testing.T) {
	var captured domain.Task
	svc := newSvc(&stubTaskRepository{
		CreateFn: func(ctx context.Context, t domain.Task) (int64, error) {
			captured = t
			return 42, nil
		},
	})

	id, err := svc.Create(context.Background(), validReq())
	assert.NoError(t, err)
	assert.Equal(t, int64(42), id)
	// 默认状态机入口
	assert.Equal(t, domain.TaskStatusCreated, captured.Status)
	assert.Equal(t, "eventual", captured.Consistency)
	// tables 序列化
	assert.Contains(t, captured.TablesJSON, `"src":"article"`)
}

func TestTaskService_Create_SourceType(t *testing.T) {
	t.Run("空 sourceType → 默认 mysql", func(t *testing.T) {
		var captured domain.Task
		svc := newSvc(&stubTaskRepository{CreateFn: func(_ context.Context, tk domain.Task) (int64, error) {
			captured = tk
			return 1, nil
		}})
		_, err := svc.Create(context.Background(), validReq()) // validReq 不设 SourceType
		assert.NoError(t, err)
		assert.Equal(t, domain.SourceTypeMySQL, captured.SourceType)
	})
	t.Run("显式 mongo → 原样持久化", func(t *testing.T) {
		var captured domain.Task
		svc := newSvc(&stubTaskRepository{CreateFn: func(_ context.Context, tk domain.Task) (int64, error) {
			captured = tk
			return 1, nil
		}})
		req := validReq()
		req.SourceType = domain.SourceTypeMongo
		_, err := svc.Create(context.Background(), req)
		assert.NoError(t, err)
		assert.Equal(t, domain.SourceTypeMongo, captured.SourceType)
	})
}

func TestTaskService_Create_RepoFailureTransparent(t *testing.T) {
	t.Run("name 重复 透传 ErrDuplicateTaskName", func(t *testing.T) {
		svc := newSvc(&stubTaskRepository{
			CreateFn: func(ctx context.Context, _ domain.Task) (int64, error) {
				return 0, errs.ErrDuplicateTaskName
			},
		})
		_, err := svc.Create(context.Background(), validReq())
		assert.ErrorIs(t, err, errs.ErrDuplicateTaskName)
	})
	t.Run("DB 异常透传", func(t *testing.T) {
		boom := errors.New("db down")
		svc := newSvc(&stubTaskRepository{
			CreateFn: func(ctx context.Context, _ domain.Task) (int64, error) {
				return 0, boom
			},
		})
		_, err := svc.Create(context.Background(), validReq())
		assert.ErrorIs(t, err, boom)
	})
}

func TestTaskService_GetList(t *testing.T) {
	t.Run("Get 透传", func(t *testing.T) {
		svc := newSvc(&stubTaskRepository{
			FindByIdFn: func(ctx context.Context, id int64) (domain.Task, error) {
				return domain.Task{Id: id, Name: "t"}, nil
			},
		})
		got, err := svc.Get(context.Background(), 7)
		assert.NoError(t, err)
		assert.Equal(t, int64(7), got.Id)
	})
	t.Run("List 透传", func(t *testing.T) {
		svc := newSvc(&stubTaskRepository{
			ListFn: func(ctx context.Context, _ repository.ListOpts) ([]domain.Task, int64, error) {
				return []domain.Task{{Id: 1}}, 1, nil
			},
		})
		list, total, err := svc.List(context.Background(), repository.ListOpts{Offset: 0, Limit: 10})
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, list, 1)
	})
}
