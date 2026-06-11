package sink

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/webook/pkg/logger"
)

// MongoSink 把 Mutation 写入 MongoDB collection。
//
// 设计：
//   - insert/update → ReplaceOne with upsert + filter {_id: PK} or {$or: [{_id: PK}, {pk_col: PK}]}
//   - delete → DeleteOne {_id: PK}
//   - Version 乐观锁通过 filter 条件 {version: {$lt: m.Version}} 实现（仅当文档不存在或旧 version < new 时才覆盖）
type MongoSink struct {
	coll     *mongo.Collection
	pkColumn string // 业务 PK 列名（如 "id"），作为 mongo doc 中的字段；同时把 PK 用作 _id 兜底
	l        logger.LoggerX
}

func NewMongoSink(coll *mongo.Collection, pkColumn string, l logger.LoggerX) Sink {
	if pkColumn == "" {
		pkColumn = "id"
	}
	return &MongoSink{coll: coll, pkColumn: pkColumn, l: l}
}

func (s *MongoSink) Apply(ctx context.Context, batch []Mutation) error {
	if len(batch) == 0 {
		return nil
	}
	var ops []mongo.WriteModel
	for _, m := range batch {
		switch m.Op {
		case OpInsert, OpUpdate:
			doc := bson.M{}
			for k, v := range m.Cols {
				doc[k] = v
			}
			doc["_id"] = m.PK
			if m.Version > 0 {
				doc["version"] = m.Version
			}
			filter := bson.M{"_id": m.PK}
			if m.Version > 0 {
				// 仅当目标 doc 不存在或 version 小于新 version 时才写
				filter = bson.M{
					"_id": m.PK,
					"$or": []bson.M{
						{"version": bson.M{"$lt": m.Version}},
						{"version": bson.M{"$exists": false}},
					},
				}
			}
			ops = append(ops, mongo.NewReplaceOneModel().
				SetFilter(filter).
				SetReplacement(doc).
				SetUpsert(true))
		case OpDelete:
			ops = append(ops, mongo.NewDeleteOneModel().
				SetFilter(bson.M{"_id": m.PK}))
		default:
			return fmt.Errorf("unknown op %q", m.Op)
		}
	}
	if _, err := s.coll.BulkWrite(ctx, ops); err != nil {
		// mongo bulkWrite 部分失败仍上抛（mongo driver 不区分软失败 / 硬失败）
		return fmt.Errorf("mongo bulk write: %w", err)
	}
	return nil
}

func (s *MongoSink) Close() error { return nil }
