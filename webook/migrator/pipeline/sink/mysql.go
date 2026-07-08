package sink

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// VersionColumn 自动启用乐观锁的版本列名。
// 当 upsert batch 中**所有行**都含此列时，SET 子句自动改为
// `col = IF(VALUES(version) > version, VALUES(col), col)`，
// 防止老 binlog 事件（旧 version）覆盖新值。
const VersionColumn = "version"

// MySQLSink 同构 MySQL 写端。
//
// 设计：
//   - Apply 把 batch 拆成 upsert（insert/update 合并）和 delete 两类，单事务执行
//   - upsert 走 INSERT ... ON DUPLICATE KEY UPDATE
//   - delete 走 DELETE WHERE pk IN (...)，幂等
//   - 乐观锁自动启用：upsertRows 全含 `version` 列时，SET 子句切换为版本条件保护，
//     防止老 binlog 事件回放覆盖新值（architecture.md §3.7 坑 1）
type MySQLSink struct {
	db        *gorm.DB
	tableName string
	pkColumn  string // 默认 "id"
	l         logger.LoggerX
}

func NewMySQLSink(db *gorm.DB, tableName, pkColumn string, l logger.LoggerX) Sink {
	if pkColumn == "" {
		pkColumn = "id"
	}
	return &MySQLSink{db: db, tableName: tableName, pkColumn: pkColumn, l: l}
}

func (s *MySQLSink) Apply(ctx context.Context, batch []Mutation) error {
	if len(batch) == 0 {
		return nil
	}
	var upsertRows []map[string]any
	var deleteIDs []string
	for _, m := range batch {
		switch m.Op {
		case OpDelete:
			deleteIDs = append(deleteIDs, m.PK)
		case OpInsert, OpUpdate:
			if m.Cols == nil {
				return fmt.Errorf("MySQLSink: mutation pk=%s op=%s has nil Cols", m.PK, m.Op)
			}
			upsertRows = append(upsertRows, m.Cols)
		default:
			return fmt.Errorf("MySQLSink: unsupported op %q (allowed: insert/update/delete)", m.Op)
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(deleteIDs) > 0 {
			if err := tx.Exec(
				fmt.Sprintf("DELETE FROM `%s` WHERE `%s` IN ?", s.tableName, s.pkColumn),
				deleteIDs).Error; err != nil {
				return fmt.Errorf("delete: %w", err)
			}
		}
		if len(upsertRows) > 0 {
			cols := collectColumns(upsertRows)
			optLock := allRowsHaveColumn(upsertRows, VersionColumn)
			assignments := buildAssignments(cols, optLock)
			if err := tx.Table(s.tableName).Clauses(clause.OnConflict{
				DoUpdates: clause.Assignments(assignments),
			}).Create(upsertRows).Error; err != nil {
				return fmt.Errorf("upsert: %w", err)
			}
		}
		return nil
	})
}

// buildAssignments 根据是否启用乐观锁构造 ON DUPLICATE KEY UPDATE 的 SET 表达式。
//
//	optLock = false（不含 version 列）：
//	    col = VALUES(col)                              -- 直接覆盖
//
//	optLock = true（所有行含 version 列）：
//	    col      = IF(VALUES(version) > version, VALUES(col), col)
//	    version  = GREATEST(version, VALUES(version))  -- 单调不回退
//
// 后者保证："新 binlog 事件覆盖旧值" 但 "旧 binlog 事件不能覆盖新值"。
func buildAssignments(cols []string, optLock bool) map[string]any {
	assignments := make(map[string]any, len(cols))
	if !optLock {
		for _, c := range cols {
			assignments[c] = gorm.Expr(fmt.Sprintf("VALUES(`%s`)", c))
		}
		return assignments
	}
	for _, c := range cols {
		if c == VersionColumn {
			continue
		}
		assignments[c] = gorm.Expr(fmt.Sprintf(
			"IF(VALUES(`%s`) > `%s`, VALUES(`%s`), `%s`)",
			VersionColumn, VersionColumn, c, c,
		))
	}
	assignments[VersionColumn] = gorm.Expr(fmt.Sprintf(
		"GREATEST(`%s`, VALUES(`%s`))",
		VersionColumn, VersionColumn,
	))
	return assignments
}

// allRowsHaveColumn 检查 rows 中每一行的 Cols 是否都含 col 字段。
// 用于乐观锁自动开关：必须所有行都带 version 才启用，避免单行无 version 时 VALUES(version) = NULL。
func allRowsHaveColumn(rows []map[string]any, col string) bool {
	if len(rows) == 0 {
		return false
	}
	for _, r := range rows {
		if _, ok := r[col]; !ok {
			return false
		}
	}
	return true
}

// Close MySQLSink 不持有独立连接（共享 ioc 注入的 *gorm.DB），关闭由 ioc 负责。
func (s *MySQLSink) Close() error {
	return nil
}

// collectColumns 收集所有 row 中出现过的列名，去重 + 排序保证 SQL 稳定（同一批 batch 多次执行 SQL 一致）。
func collectColumns(rows []map[string]any) []string {
	set := make(map[string]struct{})
	for _, r := range rows {
		for k := range r {
			set[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(set))
	for k := range set {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}
