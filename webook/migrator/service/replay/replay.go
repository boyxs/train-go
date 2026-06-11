// Package replay 是死信重放服务：把 dead_letter 表里待重放的行重新写入目标端。
//
// 流程：list 待重放死信 → 按 dl.BizTable 反查 tableIdx 路由对应 Sink →
// payload 反序列化成 Mutation → Sink.Apply → 成功 MarkReplayed / 失败 IncrementRetry 累计。
package replay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/service"
	"github.com/webook/pkg/logger"
)

// ReplayService 死信重放服务接口。
type ReplayService interface {
	// ReplayDeadLetters 重放 task 待重放死信（最多 limit 条，最老优先）；返回成功/失败计数。
	// limit <= 0 或 > 10000 回落默认 1000。
	ReplayDeadLetters(ctx context.Context, taskId int64, limit int) (replayed, failed int64, err error)
}

type InternalReplayService struct {
	taskSvc     service.TaskService
	dlRepo      repository.DeadLetterRepository
	sinkFactory sink.SinkFactory
	l           logger.LoggerX
}

func NewReplayService(
	taskSvc service.TaskService,
	dlRepo repository.DeadLetterRepository,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
) ReplayService {
	return &InternalReplayService{taskSvc: taskSvc, dlRepo: dlRepo, sinkFactory: sinkFactory, l: l}
}

func (s *InternalReplayService) ReplayDeadLetters(ctx context.Context, taskId int64, limit int) (int64, int64, error) {
	task, err := s.taskSvc.Get(ctx, taskId)
	if err != nil {
		return 0, 0, err
	}
	// ReplayDL 跨表场景：dead_letter 的 biz_table 记录的是源表（业务侧写哪张表），
	// 按 src 名反查 tableIdx，每条死信用对应表的 Sink（lazy build + 复用）。
	tables, err := task.Tables()
	if err != nil {
		return 0, 0, err
	}
	tableNameToIdx := make(map[string]int, len(tables))
	for i, tm := range tables {
		tableNameToIdx[tm.Src] = i
	}
	snkByTable := make(map[int]sink.Sink)
	getSnk := func(tableIdx int) (sink.Sink, error) {
		if v, ok := snkByTable[tableIdx]; ok {
			return v, nil
		}
		snk, berr := s.sinkFactory.BuildDst(ctx, task, tableIdx)
		if berr != nil {
			return nil, fmt.Errorf("build sink for table %d: %w", tableIdx, berr)
		}
		snkByTable[tableIdx] = snk
		return snk, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	list, err := s.dlRepo.ListUnreplayedByTask(ctx, taskId, limit)
	if err != nil {
		return 0, 0, err
	}
	var replayed, failed int64
	var okIDs []int64
	for _, dl := range list {
		var cols map[string]any
		if uerr := json.Unmarshal([]byte(dl.Payload), &cols); uerr != nil {
			s.recordFailure(ctx, dl.Id, "payload unmarshal: "+uerr.Error())
			failed++
			continue
		}
		tableIdx, ok := tableNameToIdx[dl.BizTable]
		if !ok {
			s.recordFailure(ctx, dl.Id, "biz_table "+dl.BizTable+" not in task tables")
			failed++
			continue
		}
		snk, serr := getSnk(tableIdx)
		if serr != nil {
			s.recordFailure(ctx, dl.Id, "build sink: "+serr.Error())
			failed++
			continue
		}
		mut := sink.Mutation{Op: dl.Op, Table: dl.BizTable, PK: dl.BizId, Cols: cols}
		if aerr := snk.Apply(ctx, []sink.Mutation{mut}); aerr != nil {
			s.recordFailure(ctx, dl.Id, aerr.Error())
			failed++
			continue
		}
		okIDs = append(okIDs, dl.Id)
		replayed++
	}
	if len(okIDs) > 0 {
		if merr := s.dlRepo.MarkReplayed(ctx, okIDs); merr != nil {
			s.l.Warn("MarkReplayed failed",
				logger.Int64("task_id", taskId),
				logger.Int("ids_count", len(okIDs)),
				logger.Error(merr))
		}
	}
	return replayed, failed, nil
}

// recordFailure 把单次重放失败累计到 dead_letter.retry_count + last_error。
func (s *InternalReplayService) recordFailure(ctx context.Context, dlId int64, msg string) {
	if err := s.dlRepo.IncrementRetry(ctx, dlId, msg); err != nil {
		s.l.Warn("IncrementRetry failed",
			logger.Int64("dl_id", dlId),
			logger.Error(err))
	}
}
