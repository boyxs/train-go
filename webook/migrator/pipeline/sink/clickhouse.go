package sink

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/webook/pkg/logger"
)

// ClickHouseSink 把 Mutation 写入 ClickHouse。
//
// 设计：
//   - insert / update 都走 INSERT INTO（ClickHouse 不区分；用 ReplacingMergeTree 引擎按 Version 列去重）
//   - delete 用 ALTER TABLE ... DELETE WHERE pk IN (...)（异步，注意 CK 删除不实时）
//   - Version 列由表 schema 配 ReplacingMergeTree(version) 控制乐观锁
//
// 表 schema 建议：
//
//	CREATE TABLE article (
//	  id Int64,
//	  ... other cols ...,
//	  version Int64
//	) ENGINE = ReplacingMergeTree(version) ORDER BY id;
type ClickHouseSink struct {
	conn      driver.Conn
	tableName string
	pkColumn  string
	l         logger.LoggerX
}

func NewClickHouseSink(conn driver.Conn, tableName, pkColumn string, l logger.LoggerX) Sink {
	if pkColumn == "" {
		pkColumn = "id"
	}
	return &ClickHouseSink{conn: conn, tableName: tableName, pkColumn: pkColumn, l: l}
}

func (s *ClickHouseSink) Apply(ctx context.Context, batch []Mutation) error {
	if len(batch) == 0 {
		return nil
	}
	var inserts []Mutation
	var deletePKs []string
	for _, m := range batch {
		switch m.Op {
		case OpInsert, OpUpdate:
			inserts = append(inserts, m)
		case OpDelete:
			deletePKs = append(deletePKs, m.PK)
		default:
			return fmt.Errorf("unknown op %q", m.Op)
		}
	}
	if len(inserts) > 0 {
		if err := s.bulkInsert(ctx, inserts); err != nil {
			return err
		}
	}
	if len(deletePKs) > 0 {
		if err := s.bulkDelete(ctx, deletePKs); err != nil {
			return err
		}
	}
	return nil
}

func (s *ClickHouseSink) bulkInsert(ctx context.Context, batch []Mutation) error {
	// 拿所有 column（按字典序保证稳定顺序）
	cols := collectColumnsCK(batch)
	if len(cols) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?, ", len(cols))
	placeholders = strings.TrimSuffix(placeholders, ", ")
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		s.tableName, strings.Join(cols, ", "), placeholders)
	batchExec, err := s.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("ck prepare batch: %w", err)
	}
	for _, m := range batch {
		args := make([]any, len(cols))
		for i, col := range cols {
			args[i] = m.Cols[col]
		}
		if err := batchExec.Append(args...); err != nil {
			return fmt.Errorf("ck append row pk=%s: %w", m.PK, err)
		}
	}
	if err := batchExec.Send(); err != nil {
		return fmt.Errorf("ck send batch: %w", err)
	}
	return nil
}

func (s *ClickHouseSink) bulkDelete(ctx context.Context, pks []string) error {
	// ALTER TABLE ... DELETE WHERE pk IN (...)
	placeholders := strings.Repeat("?, ", len(pks))
	placeholders = strings.TrimSuffix(placeholders, ", ")
	query := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s IN (%s)",
		s.tableName, s.pkColumn, placeholders)
	args := make([]any, len(pks))
	for i, pk := range pks {
		args[i] = pk
	}
	if err := s.conn.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("ck alter delete: %w", err)
	}
	return nil
}

func (s *ClickHouseSink) Close() error {
	return s.conn.Close()
}

// collectColumnsCK 从 batch 收集所有 column 名（按字典序），生成 INSERT 的字段列表。
// 防御性写法：不同 row 可能 column 顺序不同，统一排序后 Append。
func collectColumnsCK(batch []Mutation) []string {
	if len(batch) == 0 {
		return nil
	}
	colSet := map[string]struct{}{}
	for _, m := range batch {
		for k := range m.Cols {
			colSet[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(colSet))
	for c := range colSet {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
}
