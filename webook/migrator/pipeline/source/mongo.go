package source

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/webook/migrator/domain"
	"github.com/webook/pkg/logger"
)

// mongoScanner 抽象「按 _id 升序流式扫一个集合，逐文档回调」。
// 真实现 goMongoScanner 包 *mongo.Collection；单测用 fake 喂 canned 文档。
// 回调拿到的是已归一成 plain Go 的 map[string]any（_id 为 hex string）。
type mongoScanner interface {
	Scan(ctx context.Context, batchSize int, fn func(doc map[string]any) error) error
}

// mongoChangeEvent 是 Mongo Change Stream 事件归一后的形态（驱动无关）。
type mongoChangeEvent struct {
	Op          string         // insert / update / delete（replace 已归一为 update）
	ID          string         // documentKey._id（hex string）
	FullDoc     map[string]any // insert/update 的全文档（已归一）；delete 为 nil
	ResumeToken string         // 该事件的 resume token（_data hex）
	ClusterTime int64          // Unix 毫秒
}

// mongoWatcher 抽象「订阅 collection change stream，逐事件回调」。
// 真实现 goMongoWatcher 包 *mongo.ChangeStream；单测用 fake 喂 canned 事件。
type mongoWatcher interface {
	// Watch 从 resumeToken（空=当前位点）续订，逐事件回调 fn。
	Watch(ctx context.Context, resumeToken string, fn func(ev mongoChangeEvent) error) error
}

// MongoSource 用 Mongo collection 作迁移源，按构造函数决定角色：
//   - NewMongoSource（注入 scanner）→ FullSource：FullScan 单 shard 流式扫全集合（sort _id asc），逐文档 → Row{PK:_id hex, Cols:doc}
//   - NewMongoIncrSource（注入 watcher）→ IncrSource：IncrSubscribe 走 Change Stream（resume token 经 BinlogPos）
//   - 不实现 PKRanger（Mongo 无数值 PK 范围概念，FullEngine 兜底单 shard）
//   - Close no-op（连接生命周期由 ioc/builder 管）；nil check 防御误用（拿错接口）
//
// 文档→关系列的拍平由 transform.MongoToRelationalTransformer 负责（MongoSource 只产 Row）。
type MongoSource struct {
	scanner    mongoScanner // 全量（NewMongoSource 注入）
	watcher    mongoWatcher // 增量（NewMongoIncrSource 注入）
	collection string
	l          logger.LoggerX
}

func NewMongoSource(scanner mongoScanner, collection string, l logger.LoggerX) FullSource {
	return &MongoSource{scanner: scanner, collection: collection, l: l}
}

// NewMongoIncrSource 构造增量（Change Stream）Mongo 源。
func NewMongoIncrSource(watcher mongoWatcher, collection string, l logger.LoggerX) IncrSource {
	return &MongoSource{watcher: watcher, collection: collection, l: l}
}

// FullScan 流式扫全集合，逐文档转 Row 推 out。shard.PKMin/PKMax 不用（Mongo 单 shard）；
// shard.BatchSz 透传给底层 find batchSize。
func (s *MongoSource) FullScan(ctx context.Context, shard ShardSpec, out chan<- Row) error {
	if s.scanner == nil {
		return errors.New("MongoSource not configured for full scan (built via NewMongoIncrSource)")
	}
	return s.scanner.Scan(ctx, shard.BatchSz, func(doc map[string]any) error {
		idVal, ok := doc["_id"]
		if !ok {
			return fmt.Errorf("mongo doc in collection %q missing _id", s.collection)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- Row{Table: s.collection, PK: fmt.Sprintf("%v", idVal), Cols: doc}:
			return nil
		}
	})
}

func (s *MongoSource) IncrSubscribe(ctx context.Context, ckpt domain.Checkpoint, out chan<- ChangeEvent) error {
	if s.watcher == nil {
		return errors.New("MongoSource not configured for incremental subscribe (built via NewMongoSource)")
	}
	return s.watcher.Watch(ctx, ckpt.CursorValue, func(ev mongoChangeEvent) error {
		ce := ChangeEvent{
			Op:        ev.Op,
			Table:     s.collection,
			PK:        ev.ID,
			After:     ev.FullDoc,
			BinlogPos: ev.ResumeToken, // resume token 当游标，引擎持久化到 checkpoint
			EventTs:   ev.ClusterTime,
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- ce:
			return nil
		}
	})
}

func (s *MongoSource) Close() error { return nil }

// ── 真实现：goMongoScanner 包 *mongo.Collection ───────────────────────────

type goMongoScanner struct {
	coll *mongo.Collection
	l    logger.LoggerX
}

// NewGoMongoScanner 构造真 Mongo 扫描器（ioc 的 MongoSourceBuilder 用）。
func NewGoMongoScanner(coll *mongo.Collection, l logger.LoggerX) mongoScanner {
	return &goMongoScanner{coll: coll, l: l}
}

func (s *goMongoScanner) Scan(ctx context.Context, batchSize int, fn func(doc map[string]any) error) error {
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
	if batchSize > 0 {
		opts.SetBatchSize(int32(batchSize))
	}
	cur, err := s.coll.Find(ctx, bson.D{}, opts)
	if err != nil {
		return fmt.Errorf("mongo find: %w", err)
	}
	defer func() {
		if cerr := cur.Close(ctx); cerr != nil {
			s.l.Warn("mongo cursor close failed", logger.Error(cerr))
		}
	}()
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return fmt.Errorf("mongo decode: %w", err)
		}
		if err := fn(normalizeBSONMap(raw)); err != nil {
			return err
		}
	}
	if err := cur.Err(); err != nil {
		return fmt.Errorf("mongo cursor: %w", err)
	}
	return nil
}

// ── 真实现：goMongoWatcher 包 *mongo.ChangeStream ───────────────────────────

type goMongoWatcher struct {
	coll *mongo.Collection
	l    logger.LoggerX
}

// NewGoMongoWatcher 构造真 Mongo Change Stream 监听器（ioc 的增量 MongoSourceBuilder 用）。
// 注意：Change Stream 仅在副本集（replica set）上可用，单机 mongod 不支持。
func NewGoMongoWatcher(coll *mongo.Collection, l logger.LoggerX) mongoWatcher {
	return &goMongoWatcher{coll: coll, l: l}
}

func (w *goMongoWatcher) Watch(ctx context.Context, resumeToken string, fn func(ev mongoChangeEvent) error) (err error) {
	opts := options.ChangeStream().SetFullDocument(options.UpdateLookup)
	if resumeToken != "" {
		opts.SetResumeAfter(bson.M{"_data": resumeToken})
	}
	cs, werr := w.coll.Watch(ctx, mongo.Pipeline{}, opts)
	if werr != nil {
		return fmt.Errorf("mongo watch: %w", werr)
	}
	defer func() {
		if cce := cs.Close(context.Background()); cce != nil && err == nil {
			w.l.Warn("mongo change stream close failed", logger.Error(cce))
		}
	}()
	for cs.Next(ctx) {
		var raw struct {
			OperationType string              `bson:"operationType"`
			DocumentKey   bson.M              `bson:"documentKey"`
			FullDocument  bson.M              `bson:"fullDocument"`
			ClusterTime   primitive.Timestamp `bson:"clusterTime"`
		}
		if derr := cs.Decode(&raw); derr != nil {
			return fmt.Errorf("mongo change decode: %w", derr)
		}
		op := normalizeChangeOp(raw.OperationType)
		if op == "" {
			continue // drop / rename / invalidate 等不同步
		}
		// insert/update 但 fullDocument 为 null（UpdateLookup 时文档已被后续删除）→ 跳过：
		// 给 Sink 一个 nil Cols 的 upsert 会被拒；紧随的 delete 事件会兜底删目标行。
		if op != "delete" && raw.FullDocument == nil {
			continue
		}
		// resume token 的 _data 恒为 string；StringValueOK 兜底畸形 token 不 panic（取空串 = 从当前位点）
		tokenData, _ := cs.ResumeToken().Lookup("_data").StringValueOK()
		ev := mongoChangeEvent{
			Op:          op,
			ResumeToken: tokenData,
			ClusterTime: int64(raw.ClusterTime.T) * 1000,
		}
		if raw.DocumentKey != nil {
			ev.ID = fmt.Sprintf("%v", normalizeBSONValue(raw.DocumentKey["_id"]))
		}
		if raw.FullDocument != nil {
			ev.FullDoc = normalizeBSONMap(raw.FullDocument)
		}
		if ferr := fn(ev); ferr != nil {
			return ferr
		}
	}
	if cerr := cs.Err(); cerr != nil {
		return fmt.Errorf("mongo change stream: %w", cerr)
	}
	return nil
}

// normalizeChangeOp 把 Mongo operationType 归一到 insert/update/delete（与 BinlogEvent.Op 同枚举）；
// 不关心的事件（drop/rename/invalidate 等）返回空串，调用方跳过。
func normalizeChangeOp(op string) string {
	switch op {
	case "insert":
		return "insert"
	case "update", "replace":
		return "update"
	case "delete":
		return "delete"
	default:
		return ""
	}
}

// normalizeBSONMap 把 bson.M 递归归一成 plain Go：
// ObjectID→hex string / DateTime→Unix 毫秒 int64 / 嵌套 bson.M→map[string]any / bson.A→[]any。
func normalizeBSONMap(m bson.M) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalizeBSONValue(v)
	}
	return out
}

func normalizeBSONValue(v any) any {
	switch val := v.(type) {
	case primitive.ObjectID:
		return val.Hex()
	case primitive.DateTime:
		return int64(val) // primitive.DateTime 底层即 Unix 毫秒
	case bson.M:
		return normalizeBSONMap(val)
	case bson.A:
		out := make([]any, len(val))
		for i, e := range val {
			out[i] = normalizeBSONValue(e)
		}
		return out
	default:
		return v
	}
}
