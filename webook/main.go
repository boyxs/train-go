package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"

	"github.com/webook/ioc"
)

func main() {
	// initViper()
	// initViperV1()
	initViperV2()
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
	if err := app.Server.Run(":8089"); err != nil {
		panic(err)
	}
}

func initViper() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}
	viper.SetConfigName(env)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("config")
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
}

func initViperV1() {
	env := pflag.String("env", os.Getenv("APP_ENV"), "运行环境")
	pflag.Parse()
	if *env == "" {
		*env = "config/local.yaml"
	}
	viper.SetConfigFile(*env)
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Printf("[Config] 配置变更: %s", e.Name)
	})
	log.Printf("[Config] 加载成功: %s", viper.ConfigFileUsed())
}

func initViperV2() {
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
		Path:     "/webook",
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
