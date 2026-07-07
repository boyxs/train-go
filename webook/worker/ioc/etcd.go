package ioc

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"

	"github.com/webook/shared/confkey"
)

// InitEtcdClient etcd 客户端，供 gRPC resolver 做服务发现。读 etcd.endpoints。
func InitEtcdClient() (*etcdv3.Client, func(), error) {
	type Config struct {
		Endpoints []string `yaml:"endpoints"`
	}
	var cfg Config
	if err := viper.UnmarshalKey(confkey.Etcd, &cfg); err != nil {
		return nil, nil, err
	}
	if len(cfg.Endpoints) == 0 {
		return nil, nil, errors.New("etcd.endpoints 未配置")
	}
	cli, err := etcdv3.New(etcdv3.Config{Endpoints: cfg.Endpoints})
	if err != nil {
		return nil, nil, err
	}
	// 退出阶段 logger 可能已关，cleanup 用 stderr 兜底
	cleanup := func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "[worker] etcd client close:", err)
		}
	}
	return cli, cleanup, nil
}
