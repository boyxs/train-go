package source

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// defaultBatchSz 全量分片批量大小默认 1000；调小 → I/O 频繁，调大 → 锁/网络压力。
const defaultBatchSz = 1000

// MySQLSource 同构 MySQL 全量读端，实现 FullSource。
//
// 设计：
//   - FullScan 按 PK 单调游标分批 SELECT，避免 OFFSET 大值扫描放大；
//   - 增量用 CanalSource（binlog 解析需专用 client，不在 SQL 连接能力内）；
//   - 实现 PKRanger 暴露 PK 范围供 FullEngine 自动切片。
//
// 配置约束：tableName + pkColumn 在构造时绑定，每张表一个 MySQLSource 实例。
type MySQLSource struct {
	db        *gorm.DB
	tableName string
	pkColumn  string // 默认 "id"
	l         logger.LoggerX
}

func NewMySQLSource(db *gorm.DB, tableName, pkColumn string, l logger.LoggerX) FullSource {
	if pkColumn == "" {
		pkColumn = "id"
	}
	return &MySQLSource{db: db, tableName: tableName, pkColumn: pkColumn, l: l}
}

// FullScan 按 PK 范围 + 单调游标 + LIMIT 分批扫描。
//
//	SELECT * FROM <table> WHERE <pk> > <last_pk> AND <pk> <= <PKMax> ORDER BY <pk> LIMIT <BatchSz>
//
// 终止条件：本批返回行数 < BatchSz 或者 ctx.Done。
// 限速：QPSLimit > 0 时，每读一批 sleep len(rows)/QPSLimit 秒。
func (s *MySQLSource) FullScan(ctx context.Context, shard ShardSpec, out chan<- Row) error {
	batch := shard.BatchSz
	if batch <= 0 {
		batch = defaultBatchSz
	}
	lastPK := shard.PKMin - 1
	for {
		var rows []map[string]any
		if err := s.db.WithContext(ctx).
			Table(s.tableName).
			Where(fmt.Sprintf("`%s` > ? AND `%s` <= ?", s.pkColumn, s.pkColumn), lastPK, shard.PKMax).
			Order(fmt.Sprintf("`%s` ASC", s.pkColumn)).
			Limit(batch).
			Find(&rows).Error; err != nil {
			return fmt.Errorf("full scan shard %d: %w", shard.No, err)
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			pk, ok := toInt64(r[s.pkColumn])
			if !ok {
				s.l.Warn("full scan: skip row with non-numeric pk",
					logger.String("table", s.tableName),
					logger.String("pk_column", s.pkColumn))
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- Row{Table: s.tableName, PK: strconv.FormatInt(pk, 10), Cols: r}:
			}
			if pk > lastPK {
				lastPK = pk
			}
		}
		// 本批不足 BatchSz → 范围内已扫完
		if len(rows) < batch {
			return nil
		}
		// 限速：每秒最多 QPSLimit 行 → 本批耗时 len(rows)/QPSLimit 秒
		if shard.QPSLimit > 0 {
			sleep := time.Duration(float64(len(rows))/float64(shard.QPSLimit)*1000) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}
	}
}

// PKRange 实现 PKRanger — SELECT MIN(pk), MAX(pk) FROM table 一次拿全范围。
// 空表返回 (0, 0, nil) — 调用方按 0 范围跳过 FullScan。
func (s *MySQLSource) PKRange(ctx context.Context) (int64, int64, error) {
	type row struct {
		MinPK *int64
		MaxPK *int64
	}
	var r row
	err := s.db.WithContext(ctx).
		Table(s.tableName).
		Select(fmt.Sprintf("MIN(`%s`) AS min_pk, MAX(`%s`) AS max_pk", s.pkColumn, s.pkColumn)).
		Take(&r).Error
	if err != nil {
		return 0, 0, fmt.Errorf("pk range query: %w", err)
	}
	if r.MinPK == nil || r.MaxPK == nil {
		return 0, 0, nil
	}
	return *r.MinPK, *r.MaxPK, nil
}

// Close MySQLSource 不持有独立连接（共享 ioc 注入的 *gorm.DB），关闭由 ioc 负责。
func (s *MySQLSource) Close() error {
	return nil
}

// toInt64 把 driver 返回的不同数值类型归一到 int64。
// MySQL bigint 通过 go-sql-driver 一般是 int64；sqlmock 可能返回 []byte / string。
func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case uint64:
		return int64(x), true
	case []byte:
		n, err := strconv.ParseInt(string(x), 10, 64)
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
