package main

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"

	"github.com/webook/internal/ioc"
)

func main() {
	initViper()
	app, cleanup, err := InitWebServer()
	if err != nil {
		panic(err)
	}
	// wire 收集的 cleanup（含 OTel TracerProvider.Shutdown），进程退出前 flush 队列里的 span
	defer cleanup()
	// 后台启动 Kafka 消费者
	go func() {
		if err := app.Consumer.Start(context.Background()); err != nil {
			log.Printf("[Kafka] consumer exited: %v", err)
		}
	}()
	// 后台启动 gRPC server（供 webook-chat 等下游服务 RPC 调用）
	go func() {
		lis, err := net.Listen("tcp", app.GRPCConfig.Addr)
		if err != nil {
			log.Printf("[gRPC] listen %s 失败: %v", app.GRPCConfig.Addr, err)
			return
		}
		log.Printf("[gRPC] listening on %s", app.GRPCConfig.Addr)
		if err := app.GRPCServer.Serve(lis); err != nil {
			log.Printf("[gRPC] server exited: %v", err)
		}
	}()
	httpAddr := viper.GetString("http.addr")
	if httpAddr == "" {
		httpAddr = ":8089"
	}
	if err := app.Server.Run(httpAddr); err != nil {
		panic(err)
	}
}

func initViper() {
	env := pflag.String("env", os.Getenv("APP_ENV"), "运行环境")
	pflag.Parse()
	if *env == "" {
		*env = "config/local.yaml"
	}
	viper.SetConfigFile(*env)
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	// 环境变量覆盖 yaml 配置（L2 预埋钩子）：例如 MYSQL_DSN 覆盖 viper.GetString("mysql.dsn")
	// K8s 时代由 envFrom.secretRef 注入 Secret 的敏感字段，yaml 降级为默认值模板
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	var cfg = EtcdConfig{
		Endpoint: "http://127.0.0.1:2379",
		Path:     "/webook-core",
		Type:     "yaml",
	}
	err := viper.UnmarshalKey("etcd", &cfg)
	if err != nil {
		panic(err)
	}
	//加载远程配置
	initViperRemote(cfg)
}

func initViperRemote(cfg EtcdConfig) {
	err := viper.AddRemoteProvider("etcd3", cfg.Endpoint, cfg.Path)
	if err != nil {
		panic(err)
	}
	viper.SetConfigType(cfg.Type)
	err = viper.ReadRemoteConfig()
	if err != nil {
		log.Printf("[Config] 远程配置加载失败，使用本地配置: %v", err)
		return
	}
	log.Printf("[Config] 远程配置加载成功: endpoint=%s key=%s", cfg.Endpoint, cfg.Path)
	go func() {
		for {
			time.Sleep(5 * time.Second)
			err := viper.ReadRemoteConfig()
			if err != nil {
				log.Printf("[Config] watch 失败: %v", err)
				continue
			}
			for _, fn := range ioc.ConfigChangeCallbacks {
				fn()
			}
		}
	}()
}

type EtcdConfig struct {
	Endpoint string `yaml:"endpoint" mapstructure:"endpoint"`
	Path     string `yaml:"path" mapstructure:"path"`
	Type     string `yaml:"type" mapstructure:"type"`
}
