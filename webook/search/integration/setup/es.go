package setup

import (
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/spf13/viper"

	"github.com/boyxs/train-go/webook/pkg/logger"
	"github.com/boyxs/train-go/webook/search/repository/dao"
)

type esConfig struct {
	Addrs    []string `mapstructure:"addrs"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
}

// InitESClient 连本机 ES（读 config/test.yaml 的 data.es）。
func InitESClient() *elasticsearch.TypedClient {
	var cfg esConfig
	if err := viper.UnmarshalKey("data.es", &cfg); err != nil {
		panic("读取 data.es 配置失败: " + err.Error())
	}
	client, err := elasticsearch.NewTyped(
		elasticsearch.WithAddresses(cfg.Addrs...),
		elasticsearch.WithBasicAuth(cfg.Username, cfg.Password),
	)
	if err != nil {
		panic("初始化 ES 客户端失败: " + err.Error())
	}
	return client
}

// InitArticleDAO 用 data.es.index 指定的测试索引构造 DAO（构造时 ensureIndex 幂等建索引）。
// 集成测试的 test.yaml 应把 index 指向专用测试索引（非生产 article_v1）。
func InitArticleDAO(client *elasticsearch.TypedClient, l logger.LoggerX) dao.ArticleDAO {
	return dao.NewElasticArticleDAO(client, viper.GetString("data.es.index"), l)
}
