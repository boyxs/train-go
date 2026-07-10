package es

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
)

// Doc 是 demo 用的通用文档实体，字段刻意覆盖 ES 主要类型：
//
//	id         long                 文档主键
//	title      text                 全文分词检索（match / multi_match / 高亮）
//	category   keyword              精确匹配（term）+ 分组聚合（terms）+ collapse 折叠
//	tags       keyword[]            多值精确匹配（terms）
//	score      double               范围/排序/指标聚合/function_score/script_score
//	views      integer              数值字段（function_score field_value_factor）
//	content    text                 全文分词 + 高亮
//	created_at date(epoch_millis)   时间范围/排序（遵循项目时间铁律：Unix 毫秒 int64）
type Doc struct {
	Id        int64    `json:"id"`
	Title     string   `json:"title"`
	Category  string   `json:"category"`
	Tags      []string `json:"tags"`
	Score     float64  `json:"score"`
	Views     int      `json:"views"`
	Content   string   `json:"content"`
	CreatedAt int64    `json:"created_at"`
}

// docId ES 文档 _id 用字符串，这里用数值 Id 转字符串。
func docId(id int64) string { return strconv.FormatInt(id, 10) }

// decodeDoc 把 _source 原始 JSON 解析成 Doc。
func decodeDoc(raw json.RawMessage) (Doc, error) {
	if len(raw) == 0 {
		return Doc{}, nil
	}
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		return Doc{}, fmt.Errorf("unmarshal doc source: %w", err)
	}
	return d, nil
}

// indexMappingJSON 是索引的 settings + mapping。用 //go:embed 从 doc_index.json 读，
// 避免把 JSON 硬写进 Go 代码。单分片零副本便于单机 demo 立即可见。
//
//go:embed doc_index.json
var indexMappingJSON []byte
