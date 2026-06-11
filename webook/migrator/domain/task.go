package domain

import (
	"encoding/json"
	"fmt"
)

type TaskStatus int8

const (
	TaskStatusCreated     TaskStatus = 0
	TaskStatusFullRunning TaskStatus = 1
	TaskStatusFullDone    TaskStatus = 2
	TaskStatusIncrRunning TaskStatus = 3
	TaskStatusSwitched    TaskStatus = 5
	TaskStatusFailed      TaskStatus = -1
)

type Mode string

const (
	ModeDualWrite Mode = "dual_write"
	ModeCDC       Mode = "cdc"
)

// Valid 判断 Mode 是否合法，service 层校验入参时使用。
func (m Mode) Valid() bool {
	switch m {
	case ModeDualWrite, ModeCDC:
		return true
	}
	return false
}

type Kind string

const (
	KindCrossDC       Kind = "cross_dc"
	KindSharding      Kind = "sharding"
	KindSchema        Kind = "schema"
	KindHeterogeneous Kind = "heterogeneous"
)

// Valid 判断 Kind 是否合法。
func (k Kind) Valid() bool {
	switch k {
	case KindCrossDC, KindSharding, KindSchema, KindHeterogeneous:
		return true
	}
	return false
}

type SourceType string

const (
	SourceTypeMySQL SourceType = "mysql"
	SourceTypeMongo SourceType = "mongo"
)

// Valid 判断 SourceType 是否合法。空值不合法（先用 Normalize 兜底默认再校验）。
func (s SourceType) Valid() bool {
	switch s {
	case SourceTypeMySQL, SourceTypeMongo:
		return true
	}
	return false
}

// Normalize 把空 SourceType 归一到默认 mysql（不传 sourceType 的请求按 MySQL 源处理）。
// 非空值原样返回，合法性交给 Valid 判断。
func (s SourceType) Normalize() SourceType {
	if s == "" {
		return SourceTypeMySQL
	}
	return s
}

// TableMapping 描述一张源表 → 目标表的映射规则。
// 一个 task 可以挂多张表，序列化后存在 task.tables_json。
type TableMapping struct {
	Src              string   `json:"src"`                        // 源表名
	Dst              string   `json:"dst"`                        // 目标表名
	PartitionKey     string   `json:"partitionKey"`               // 分区 / 分片键，默认 "id"
	Filter           string   `json:"filter,omitempty"`           // SQL where 过滤条件，如 "deleted_at = 0"
	Transform        string   `json:"transform,omitempty"`        // 自定义 transform 名（已注册）
	SensitiveColumns []string `json:"sensitiveColumns,omitempty"` // 对账日志需 mask 的字段
}

type Task struct {
	Id           int64
	Name         string
	Mode         Mode
	Kind         Kind
	SourceType   SourceType
	SourceDsnRef string
	SinkType     string
	SinkDsnRef   string
	TablesJSON   string
	Status       TaskStatus
	GrayPercent  int16
	Consistency  string
	CreatedAt    int64
	UpdatedAt    int64
}

// Tables 反序列化 TablesJSON → TableMapping 数组，归一化每张表的 PartitionKey（兜底 "id"）。
// 任意 src/dst 缺失或 JSON 损坏返回 error。
//
// 一个 task 可承载多张表的迁移（handler 按表 fan-out 启动多个引擎子任务），
// 每张表独立 checkpoint（shard_no 编码 tableIdx * shardStride + realShardNo）。
func (t Task) Tables() ([]TableMapping, error) {
	if t.TablesJSON == "" {
		return nil, fmt.Errorf("task %d tables_json is empty", t.Id)
	}
	var tables []TableMapping
	if err := json.Unmarshal([]byte(t.TablesJSON), &tables); err != nil {
		return nil, fmt.Errorf("task %d tables_json unmarshal: %w", t.Id, err)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("task %d tables_json has no entries", t.Id)
	}
	for i := range tables {
		if tables[i].PartitionKey == "" {
			tables[i].PartitionKey = "id"
		}
		if tables[i].Src == "" || tables[i].Dst == "" {
			return nil, fmt.Errorf("task %d tables[%d] missing src/dst (got src=%q dst=%q)", t.Id, i, tables[i].Src, tables[i].Dst)
		}
	}
	return tables, nil
}

// PickTable 取指定下标的 TableMapping（已归一化）。
func (t Task) PickTable(idx int) (TableMapping, error) {
	tables, err := t.Tables()
	if err != nil {
		return TableMapping{}, err
	}
	if idx < 0 || idx >= len(tables) {
		return TableMapping{}, fmt.Errorf("task %d table index %d out of range [0, %d)", t.Id, idx, len(tables))
	}
	return tables[idx], nil
}

// ShardStride checkpoint shard_no 编码：encoded = tableIdx * ShardStride + realShardNo。
// 每张表最多 ShardStride 个分片（10000 足够生产用，最多 ~214748 张表，受 int32 上限）。
const ShardStride = 10000

// EncodeShardNo 编码 (tableIdx, shardNo) → checkpoint.shard_no。
func EncodeShardNo(tableIdx, shardNo int) int32 {
	return int32(tableIdx*ShardStride + shardNo)
}

// DecodeShardNo 反向解析 checkpoint.shard_no → (tableIdx, shardNo)。
func DecodeShardNo(encoded int32) (tableIdx, shardNo int) {
	e := int(encoded)
	return e / ShardStride, e % ShardStride
}
