package source

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/boyxs/train-go/webook/migrator/consts"
	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// BinlogEvent 底层 binlog 事件抽象。
//
// 由 BinlogClient 推送（真实现：go-mysql canal client；测试：stub）。
// 不直接暴露给上层，CanalSource 转成 ChangeEvent 后送 out chan。
type BinlogEvent struct {
	Op        string // insert / update / delete
	Table     string
	PK        string
	Before    map[string]any
	After     map[string]any
	BinlogPos string // 格式 "file/pos"，与 checkpoint cursor_value 对齐
	GTID      string
	EventTs   int64 // 毫秒
}

// BinlogClient 抽象 canal-go / maxwell / debezium 等 SDK 的核心能力。
//
// 实现矩阵：
//
//	GoMySQLCanalClient   真 SDK（go-mysql-org/go-mysql/canal），订阅真实 binlog stream
//	stubBinlogClient     测试专用，注入假事件
type BinlogClient interface {
	// Subscribe 从指定 binlog 位点续订；返回事件 channel。
	// ctx cancel 后 channel 必须被关闭（让上层 select 退出循环）。
	//
	// fromPos 格式 "file/pos"；空字符串 → 从 master 当前位点起订（首次订阅）。
	// GTID 模式未实现，CanalSource 上层会显式拒绝 cursor_kind=gtid 的 checkpoint。
	Subscribe(ctx context.Context, fromPos string) (<-chan BinlogEvent, error)

	// Stop 关闭底层连接（不影响已订阅的 ctx），用于 graceful shutdown。
	Stop() error
}

// CanalSource binlog 增量读端，实现 IncrSource。
//
// 设计：
//   - 持有 BinlogClient（wire 注入；真集成时换 GoMySQLCanalClient）
//   - IncrSubscribe 按 checkpoint.CursorKind 解析续订位点 → 把 BinlogEvent 转 ChangeEvent 推到 out chan
//   - 全量用 MySQLSource（CanalSource 只管增量）
//   - 实现 LagReporter（基于最近一次事件时间戳）
//   - 构造函数返回 IncrSource 接口
type CanalSource struct {
	client BinlogClient
	l      logger.LoggerX

	// lastEventTs 缓存每个任务最近事件时间戳（毫秒），供 Lag 计算。
	lastEventTs sync.Map // map[int64]int64
}

func NewCanalSource(client BinlogClient, l logger.LoggerX) IncrSource {
	return &CanalSource{client: client, l: l}
}

// IncrSubscribe 按 checkpoint 续订 binlog，转 ChangeEvent 推到 out。
//
// 调用方负责 close(out)；本函数在 ctx cancel / client chan close 时返回。
//
// GTID checkpoint 显式不支持：BinlogClient 当前只走 binlog file/pos 续订模式（go-mysql canal 主推）。
// 真要支持 GTID 续订需要 BinlogClient 实现侧重写 + 接口加 fromGTID 参数。
func (s *CanalSource) IncrSubscribe(ctx context.Context, ckpt domain.Checkpoint, out chan<- ChangeEvent) error {
	var fromPos string
	switch ckpt.CursorKind {
	case consts.CursorKindBinlog:
		fromPos = ckpt.CursorValue
	case consts.CursorKindGTID:
		return fmt.Errorf("CanalSource: GTID cursor kind not yet implemented; use binlog_pos checkpoint or wait for GTID support")
	case "":
		// 首次订阅：fromPos 为空，由底层 client 决定起点（通常 master 当前位点）
	default:
		return fmt.Errorf("CanalSource: unsupported cursor kind %q (allowed: binlog_pos)", ckpt.CursorKind)
	}

	events, err := s.client.Subscribe(ctx, fromPos)
	if err != nil {
		return fmt.Errorf("canal subscribe: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-events:
			if !ok {
				// client 关闭 channel — 优雅退出（ctx cancel 路径上来）
				return ctx.Err()
			}
			s.lastEventTs.Store(ckpt.TaskId, e.EventTs)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- e:
			}
		}
	}
}

// Close 关闭底层 binlog client。
func (s *CanalSource) Close() error {
	return s.client.Stop()
}

// Lag 实现 LagReporter — 返回 now - lastEventTs（毫秒）；任务尚无事件返回 -1。
func (s *CanalSource) Lag(taskId int64) int64 {
	v, ok := s.lastEventTs.Load(taskId)
	if !ok {
		return -1
	}
	ts, ok := v.(int64)
	if !ok {
		// 异常类型（sync.Map 误存非 int64）视为无效，与未存在等价
		return -1
	}
	return time.Now().UnixMilli() - ts
}

// ParseBinlogPos 把 "file/pos" 还原回 (file, pos, ok)；不合法格式 ok=false。
func ParseBinlogPos(s string) (string, int64, bool) {
	idx := strings.LastIndex(s, "/")
	if idx == -1 {
		return s, 0, false
	}
	pos, err := strconv.ParseInt(s[idx+1:], 10, 64)
	if err != nil {
		return s[:idx], 0, false
	}
	return s[:idx], pos, true
}
