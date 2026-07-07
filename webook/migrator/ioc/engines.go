package ioc

import (
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/IBM/sarama"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/webook/migrator/domain"
	"github.com/webook/migrator/pipeline/dsn"
	"github.com/webook/migrator/pipeline/sink"
	"github.com/webook/migrator/pipeline/source"
	"github.com/webook/migrator/pipeline/transform"
	"github.com/webook/migrator/repository"
	"github.com/webook/migrator/service"
	"github.com/webook/migrator/service/full"
	"github.com/webook/migrator/service/incr"
	"github.com/webook/migrator/service/verify"
	"github.com/webook/pkg/logger"
	"github.com/webook/shared/confkey"
)

// InitDBResolver 注入 dsn.Resolver。
//
// 当前用 StaticResolver 占位：src/dst 都返回 ioc 装的控制库 db（本地演示模式）。
// 生产升级方向：实现 Vault/K8s Secret 解析 + per-task 连接池；ioc 层换实现即可，上层无感。
func InitDBResolver(db *gorm.DB) dsn.Resolver {
	return dsn.NewStaticResolver(db)
}

// InitSourceFactory 注入 SourceFactory（MySQL + Canal + ES + Mongo + DSN Resolver）。
// BuildFullSrc 按 task.SourceType 分发 MySQL/Mongo；BuildIncrSrc mysql 源 cdc→Canal；
// BuildDst 按 task.SinkType 分发 MySQL/ES/Mongo（异构对账关键路径，mongo 分支复用 MongoSource 读 dst collection）。
func InitSourceFactory(db *gorm.DB, l logger.LoggerX, resolver dsn.Resolver, mongoClient *mongo.Client) source.SourceFactory {
	return source.NewSourceFactory(db, l,
		source.WithCanalClient(buildCanalClient(l)),
		source.WithDBResolver(resolver),
		source.WithESSourceBuilder(buildESSourceBuilder(l)),
		source.WithMongoSourceBuilder(buildMongoSourceBuilder(mongoClient, l)),
		source.WithMongoIncrSourceBuilder(buildMongoIncrSourceBuilder(mongoClient, l)),
	)
}

// buildESSourceBuilder 返回 yaml-driven ES Source builder：按 migrator.es.{addrs,username,password} 构造 ESSource。
// 与 buildESSink 共享 buildESClient（避免 dst 端读/写连不同集群或认证不一致）。
func buildESSourceBuilder(l logger.LoggerX) source.ESSourceBuilder {
	return func(indexName, pkField string) (source.FullSource, error) {
		client, err := buildESClient()
		if err != nil {
			return nil, fmt.Errorf("es client: %w", err)
		}
		return source.NewESSource(client, indexName, pkField, l), nil
	}
}

// buildCanalClient 返回 yaml-driven canal client builder：每个 cdc task 启动时按全局 canal 配置 + task tables 构造 GoMySQLCanalClient。
//
// 配置 yaml 段：
//
//	migrator:
//	  canal:
//	    addr: "webook-mysql:3306"
//	    user: "canal"
//	    password: "canal"
//	    serverIdBase: 1001     # 实际 ServerID = serverIdBase + task.Id（避免多 task 撞 ServerID）
//	    flavor: "mysql"
func buildCanalClient(l logger.LoggerX) func(task domain.Task) (source.BinlogClient, error) {
	return func(task domain.Task) (source.BinlogClient, error) {
		addr := viper.GetString("migrator.canal.addr")
		user := viper.GetString("migrator.canal.user")
		password := viper.GetString("migrator.canal.password")
		serverIdBase := viper.GetUint32("migrator.canal.server_id_base")
		flavor := viper.GetString("migrator.canal.flavor")
		if addr == "" {
			return nil, fmt.Errorf("migrator.canal.addr 未配置")
		}
		if serverIdBase == 0 {
			serverIdBase = 1001
		}
		// 按 task.Tables() 构造 includeTableRegex
		tables, err := task.Tables()
		if err != nil {
			return nil, err
		}
		regex := make([]string, 0, len(tables))
		dbName := extractDBName(viper.GetString(confkey.DataMySQLDSN))
		for _, tm := range tables {
			// 严格匹配 dbName.tableName
			regex = append(regex, fmt.Sprintf(`%s\.%s`, dbName, tm.Src))
		}
		return source.NewGoMySQLCanalClient(source.GoMySQLCanalClientConfig{
			Addr:              addr,
			User:              user,
			Password:          password,
			ServerID:          serverIdBase + uint32(task.Id),
			Flavor:            flavor,
			IncludeTableRegex: regex,
			BufSize:           viper.GetInt("migrator.incr.channel_buf"),
		}, l)
	}
}

// extractDBName 从 mysql DSN（DSN 形如 `user:pass@tcp(host:port)/dbname?...`）解析数据库名。
// 解析失败兜底返 "webook"（与默认业务库名对齐）。
func extractDBName(dsn string) string {
	// 找 ")/" 后面到 "?" 或结尾
	idx := strings.LastIndex(dsn, ")/")
	if idx == -1 {
		return "webook"
	}
	rest := dsn[idx+2:]
	q := strings.Index(rest, "?")
	if q == -1 {
		return rest
	}
	return rest[:q]
}

// InitSinkFactory 注入 SinkFactory（MySQL + 异构 ES/CK/Mongo/Kafka + DSN Resolver）。
// InternalSinkFactory 持 heteroBuilder，按 task.SinkType 分发到对应 Sink 实现。
func InitSinkFactory(db *gorm.DB, l logger.LoggerX, resolver dsn.Resolver, mongoClient *mongo.Client) sink.SinkFactory {
	return sink.NewSinkFactory(db, l,
		sink.WithHeteroBuilder(buildHeteroSink(mongoClient, l)),
		sink.WithDBResolver(resolver),
	)
}

// buildHeteroSink 返回异构 sink builder：按 task.SinkType 创建对应 Sink。
// 配置 yaml 段：
//
//	migrator:
//	  es:
//	    addrs: ["http://webook-es:9200"]
//	  clickhouse:
//	    addr: "webook-ck:9000"
//	    database: "webook"
//	    user: "default"
//	    password: ""
//	  mongo:
//	    uri: "mongodb://webook-mongo:27017"
//	    database: "webook"
//	  kafka:
//	    brokers: ["webook-kafka:9092"]
func buildHeteroSink(mongoClient *mongo.Client, l logger.LoggerX) sink.HeteroSinkBuilder {
	return func(task domain.Task, tm domain.TableMapping) (sink.Sink, error) {
		switch task.SinkType {
		case "es", "elasticsearch":
			return buildESSink(tm.Dst, l)
		case "clickhouse", "ck":
			return buildCKSink(tm.Dst, tm.PartitionKey, l)
		case "mongo", "mongodb":
			return buildMongoSink(mongoClient, tm.Dst, tm.PartitionKey, l)
		case "kafka":
			return buildKafkaSink(tm.Dst, l)
		default:
			return nil, fmt.Errorf("unsupported sink_type %q", task.SinkType)
		}
	}
}

// buildESClient 共享给 ESSink + ESSourceBuilder 的客户端构造（认证字段两侧一致）。
// yaml:
//
//	migrator.es.addrs:    ["http://localhost:9200"]
//	migrator.es.username: "elastic"   # 可选;生产 xpack.security 开启时必填
//	migrator.es.password: "xxx"       # 可选
func buildESClient() (*elasticsearch.Client, error) {
	addrs := viper.GetStringSlice("migrator.es.addrs")
	if len(addrs) == 0 {
		return nil, fmt.Errorf("migrator.es.addrs 未配置")
	}
	return elasticsearch.NewClient(elasticsearch.Config{
		Addresses: addrs,
		Username:  viper.GetString("migrator.es.username"),
		Password:  viper.GetString("migrator.es.password"),
	})
}

func buildESSink(indexName string, l logger.LoggerX) (sink.Sink, error) {
	client, err := buildESClient()
	if err != nil {
		return nil, fmt.Errorf("es client: %w", err)
	}
	return sink.NewESSink(client, indexName, l), nil
}

func buildCKSink(tableName, pkColumn string, l logger.LoggerX) (sink.Sink, error) {
	addr := viper.GetString("migrator.clickhouse.addr")
	if addr == "" {
		return nil, fmt.Errorf("migrator.clickhouse.addr 未配置")
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: viper.GetString("migrator.clickhouse.database"),
			Username: viper.GetString("migrator.clickhouse.user"),
			Password: viper.GetString("migrator.clickhouse.password"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	return sink.NewClickHouseSink(conn, tableName, pkColumn, l), nil
}

// mongoCollection 按共享 client（ioc.InitMongoClient 注入）+ migrator.mongo.database 取 collection。
// client=nil（未配 migrator.mongo.uri 或连接失败）时返 error，由调用方在引擎启动期上抛。
func mongoCollection(client *mongo.Client, collection string) (*mongo.Collection, error) {
	if client == nil {
		return nil, fmt.Errorf("migrator.mongo.uri 未配置")
	}
	dbName := viper.GetString("migrator.mongo.database")
	return client.Database(dbName).Collection(collection), nil
}

func buildMongoSink(client *mongo.Client, collName, pkColumn string, l logger.LoggerX) (sink.Sink, error) {
	coll, err := mongoCollection(client, collName)
	if err != nil {
		return nil, err
	}
	return sink.NewMongoSink(coll, pkColumn, l), nil
}

// buildMongoSourceBuilder 返回 Mongo 全量 Source builder（按 collection 拿一个 *mongo.Collection 句柄，
// 共享同一 client 的连接池）。pkField 不用（Mongo PK 恒为 _id）。
func buildMongoSourceBuilder(client *mongo.Client, l logger.LoggerX) source.MongoSourceBuilder {
	return func(collection, _ string) (source.FullSource, error) {
		coll, err := mongoCollection(client, collection)
		if err != nil {
			return nil, err
		}
		return source.NewMongoSource(source.NewGoMongoScanner(coll, l), collection, l), nil
	}
}

// buildMongoIncrSourceBuilder 返回 Mongo 增量 Source builder（Change Stream）。pkField 不用（PK 恒为 _id）。
func buildMongoIncrSourceBuilder(client *mongo.Client, l logger.LoggerX) source.MongoIncrSourceBuilder {
	return func(collection, _ string) (source.IncrSource, error) {
		coll, err := mongoCollection(client, collection)
		if err != nil {
			return nil, err
		}
		return source.NewMongoIncrSource(source.NewGoMongoWatcher(coll, l), collection, l), nil
	}
}

func buildKafkaSink(topic string, l logger.LoggerX) (sink.Sink, error) {
	brokers := viper.GetStringSlice("migrator.kafka.brokers")
	if len(brokers) == 0 {
		return nil, fmt.Errorf("migrator.kafka.brokers 未配置")
	}
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Partitioner = sarama.NewHashPartitioner // key=PK → 同 PK 同 partition 保单行顺序
	producer, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	return sink.NewKafkaSink(producer, topic, l), nil
}

// InitFullEngine FullEngine 持 factory 按 task 动态 build src/snk；批量大小从 yaml 读。
func InitFullEngine(
	ts service.TaskService,
	ckptRepo repository.CheckpointRepository,
	srcFactory source.SourceFactory,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
) full.FullEngine {
	return full.NewFullEngine(ts, ckptRepo, srcFactory, sinkFactory, l, buildTransformRegistry(), full.Config{
		BatchSize:  viper.GetInt("migrator.full.batch_size"),
		ChannelBuf: viper.GetInt("migrator.full.channel_buf"),
	})
}

// buildTransformRegistry 构造 transform 注册表并注册内置 transformer（full / incr 引擎共用）。
func buildTransformRegistry() *transform.Registry {
	reg := transform.NewRegistry()
	reg.Register(transform.TransformMongoToRelational, transform.MongoToRelationalTransformer{})
	return reg
}

// InitIncrEngine IncrEngine 持 factory 按 task 动态 build src/snk。
//
// partitionCount 默认 1（单 worker 串行）；多核 / 大流量场景按 CPU + 业务峰值 qps 配 4 / 8 / 16；
// 上限受限于 MySQLSink 的连接池容量（每个 partition 一个 worker goroutine 共用 *gorm.DB 连接池）。
//
// 接 InternalSourceFactory 时 IncrSubscribe 返 ErrIncrNotSupported（MySQL 不订阅 binlog）；
// 真实增量需要 factory 内部按 task.Mode 切到 CanalSource。
func InitIncrEngine(
	ts service.TaskService,
	ckptRepo repository.CheckpointRepository,
	srcFactory source.SourceFactory,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
) incr.IncrEngine {
	return incr.NewIncrEngine(ts, ckptRepo, srcFactory, sinkFactory, l, buildTransformRegistry(), incr.Config{
		BatchSize:      viper.GetInt("migrator.incr.batch_size"),
		ChannelBuf:     viper.GetInt("migrator.incr.channel_buf"),
		PartitionCount: viper.GetInt("migrator.incr.partition_count"),
		FlushInterval:  viper.GetDuration("migrator.incr.flush_interval"),
	})
}

// InitVerifyEngine VerifyEngine 持 factory 按 task 构造双向 src/dst Source + 双向 Sink（Repair overwrite 写）。
func InitVerifyEngine(
	ts service.TaskService,
	validateRepo repository.ValidateLogRepository,
	srcFactory source.SourceFactory,
	sinkFactory sink.SinkFactory,
	l logger.LoggerX,
) verify.VerifyEngine {
	return verify.NewVerifyEngine(
		ts, validateRepo,
		srcFactory, sinkFactory,
		l, buildTransformRegistry(),
		verify.Config{
			BatchSize:  viper.GetInt("migrator.verify.batch_size"),
			ChannelBuf: viper.GetInt("migrator.verify.channel_buf"),
		},
	)
}
