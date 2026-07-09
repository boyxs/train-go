package redisx

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 单机 + 高级配置：应返回 *redis.Client 且 Options() 映射全部字段。
func TestNewClient_SingleMapsAdvancedOpts(t *testing.T) {
	c := Config{
		Mode: "single", Addr: "127.0.0.1:6379", Password: "pw",
		PoolSize: 5, MinIdleConns: 2,
		DialTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
		MaxRetries: -1, ContextTimeoutEnabled: true,
	}
	cli := NewClient(c)
	t.Cleanup(func() { _ = cli.Close() })

	rc, ok := cli.(*redis.Client)
	require.True(t, ok, "single mode 应返回 *redis.Client")
	o := rc.Options()
	assert.Equal(t, "127.0.0.1:6379", o.Addr)
	assert.Equal(t, "pw", o.Password)
	assert.Equal(t, 5, o.PoolSize)
	assert.Equal(t, 2, o.MinIdleConns)
	assert.Equal(t, 2*time.Second, o.DialTimeout)
	assert.Equal(t, time.Second, o.ReadTimeout)
	assert.True(t, o.ContextTimeoutEnabled)
	assert.Equal(t, 0, o.MaxRetries, "MaxRetries=-1 经 go-redis init 归一为 0（关闭重试）")
}

// 集群 + 高级配置：应返回 *redis.ClusterClient 且 Options() 映射 Addrs / 池 / 集群路由。
func TestNewClient_ClusterMapsAdvancedOpts(t *testing.T) {
	c := Config{
		Mode: "cluster", Addrs: []string{"127.0.0.1:7001", "127.0.0.1:7002"}, Password: "pw",
		PoolSize: 7, MaxRedirects: 5, RouteRandomly: true,
	}
	cli := NewClient(c)
	t.Cleanup(func() { _ = cli.Close() })

	cc, ok := cli.(*redis.ClusterClient)
	require.True(t, ok, "cluster mode 应返回 *redis.ClusterClient")
	o := cc.Options()
	assert.Equal(t, []string{"127.0.0.1:7001", "127.0.0.1:7002"}, o.Addrs)
	assert.Equal(t, 7, o.PoolSize)
	assert.Equal(t, 5, o.MaxRedirects)
	assert.True(t, o.RouteRandomly)
}

// 空 mode 默认单机（向后兼容：现有 yaml 只有 addr/password）。
func TestNewClient_DefaultModeIsSingle(t *testing.T) {
	cli := NewClient(Config{Addr: "127.0.0.1:6379"})
	t.Cleanup(func() { _ = cli.Close() })
	_, ok := cli.(*redis.Client)
	assert.True(t, ok, "空 mode 应默认单机")
}
