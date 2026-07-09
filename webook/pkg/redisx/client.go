// Package redisx 收敛全仓统一的 redis 连接 + 高级配置与 client 构建（单机 / 集群），
// 各服务 ioc / 测试 setup 不再各写一份内联 Config{Addr,Password}。
package redisx

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// Config 统一的 redis 连接 + 高级配置（单机 + 集群）。5 份 yaml 的 data.redis 段就地 unmarshal 进它。
// 零值字段回落 go-redis 默认（MaxRetries=0→3、PoolSize=0→10×CPU、超时各有默认），故 yaml 只写要调的键。
// 时间值用 duration 字符串（"2s" / "500ms"），viper 自动转 time.Duration。
type Config struct {
	Mode     string   `mapstructure:"mode"`     // single(默认/空) | cluster
	Addr     string   `mapstructure:"addr"`     // 单机地址
	Addrs    []string `mapstructure:"addrs"`    // 集群节点地址
	Password string   `mapstructure:"password"` // 密码，走 ${REDIS_PASS}

	// 连接池
	PoolSize        int           `mapstructure:"pool_size"`          // 每节点最大连接数，0=10×CPU
	MinIdleConns    int           `mapstructure:"min_idle_conns"`     // 预热常温连接数，降周期性获取的冷启延迟
	PoolTimeout     time.Duration `mapstructure:"pool_timeout"`       // 池满时等连接的上限
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"` // 空闲连接回收
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`  // 连接最大存活

	// 超时
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`  // 建连超时，0=5s
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`  // 读超时，0=3s
	WriteTimeout time.Duration `mapstructure:"write_timeout"` // 写超时，0=ReadTimeout

	// 重试
	MaxRetries      int           `mapstructure:"max_retries"`       // 0=3；-1=关闭（锁专用：acquire 非幂等，重试会重复计数）
	MinRetryBackoff time.Duration `mapstructure:"min_retry_backoff"` // 重试退避下限
	MaxRetryBackoff time.Duration `mapstructure:"max_retry_backoff"` // 重试退避上限

	// 行为
	ContextTimeoutEnabled bool `mapstructure:"context_timeout_enabled"` // ctx deadline 作用到 I/O

	// 集群专属
	MaxRedirects   int  `mapstructure:"max_redirects"`    // MOVED/ASK 重定向次数，0=3
	ReadOnly       bool `mapstructure:"read_only"`        // 允许从库读
	RouteByLatency bool `mapstructure:"route_by_latency"` // 按延迟就近路由（读）
	RouteRandomly  bool `mapstructure:"route_randomly"`   // 随机路由（读）
}

// NewClient 按 Mode 建单机 / 集群 UniversalClient，映射全部高级配置；零值回落 go-redis 默认。
func NewClient(c Config) redis.UniversalClient {
	if c.Mode == "cluster" {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:                 c.Addrs,
			Password:              c.Password,
			PoolSize:              c.PoolSize,
			MinIdleConns:          c.MinIdleConns,
			PoolTimeout:           c.PoolTimeout,
			ConnMaxIdleTime:       c.ConnMaxIdleTime,
			ConnMaxLifetime:       c.ConnMaxLifetime,
			DialTimeout:           c.DialTimeout,
			ReadTimeout:           c.ReadTimeout,
			WriteTimeout:          c.WriteTimeout,
			MaxRetries:            c.MaxRetries,
			MinRetryBackoff:       c.MinRetryBackoff,
			MaxRetryBackoff:       c.MaxRetryBackoff,
			ContextTimeoutEnabled: c.ContextTimeoutEnabled,
			MaxRedirects:          c.MaxRedirects,
			ReadOnly:              c.ReadOnly,
			RouteByLatency:        c.RouteByLatency,
			RouteRandomly:         c.RouteRandomly,
		})
	}
	return redis.NewClient(&redis.Options{
		Addr:                  c.Addr,
		Password:              c.Password,
		PoolSize:              c.PoolSize,
		MinIdleConns:          c.MinIdleConns,
		PoolTimeout:           c.PoolTimeout,
		ConnMaxIdleTime:       c.ConnMaxIdleTime,
		ConnMaxLifetime:       c.ConnMaxLifetime,
		DialTimeout:           c.DialTimeout,
		ReadTimeout:           c.ReadTimeout,
		WriteTimeout:          c.WriteTimeout,
		MaxRetries:            c.MaxRetries,
		MinRetryBackoff:       c.MinRetryBackoff,
		MaxRetryBackoff:       c.MaxRetryBackoff,
		ContextTimeoutEnabled: c.ContextTimeoutEnabled,
	})
}
