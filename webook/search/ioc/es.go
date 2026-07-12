package ioc

import (
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/search/repository/dao"
)

// esConfig ES 连接配置：addrs 多节点即集群；username/password 走 ${ES_PASS} 连启用 xpack.security 的 ES。
// 目标 ES 未开安全时带凭据无害（服务端忽略），故 5 份 yaml 一律带 auth。
// 注：data.es.index 是逻辑别名（默认 article），由 InitArticleDAO 单独读取，不在此结构。
type esConfig struct {
	Addrs    []string `mapstructure:"addrs"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
}

func loadESConfig() esConfig {
	var cfg esConfig
	if err := viper.UnmarshalKey("data.es", &cfg); err != nil {
		panic("读取 data.es 配置失败: " + err.Error())
	}
	if len(cfg.Addrs) == 0 {
		panic("data.es.addrs 未配置")
	}
	return cfg
}

func InitESClient(l logger.LoggerX) *elasticsearch.TypedClient {
	cfg := loadESConfig()
	// go-elasticsearch v9 起用函数式 Option（NewTypedClient/Config 已弃用）。
	client, err := elasticsearch.NewTyped(
		elasticsearch.WithAddresses(cfg.Addrs...),
		elasticsearch.WithBasicAuth(cfg.Username, cfg.Password),
	)
	if err != nil {
		panic("初始化 ES 客户端失败: " + err.Error())
	}
	return client
}

// InitArticleDAO 用配置里的逻辑索引「别名」构造 DAO（DAO 构造时 ensureIndex 幂等确保 alias→物理版本索引）。
// data.es.index 是稳定别名（如 article），物理索引 article_v1 由 ensureIndex 建并挂别名；为空兜底 article。
func InitArticleDAO(client *elasticsearch.TypedClient, l logger.LoggerX) dao.ArticleDAO {
	alias := viper.GetString("data.es.index")
	if alias == "" {
		alias = "article"
	}
	return dao.NewElasticArticleDAO(client, alias, l)
}
