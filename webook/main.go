package main

import (
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"
	"go.uber.org/zap"
)

func main() {
	// initViper()
	// initViperV1()
	initViperV2()
	initLogger()
	server := InitWebServer()
	err := server.Run(":8089")
	if err != nil {
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
		*env = "config/dev.yaml"
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
		*env = "config/dev.yaml"
	}
	viper.SetConfigFile(*env)
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
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
	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Printf("[Config] 远程配置变更: %s", e.Name)
	})
	go func() {
		for {
			err := viper.WatchRemoteConfig()
			if err != nil {
				log.Printf("[Config] watch 失败: %v", err)
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

func initLogger() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
}

type EtcdConfig struct {
	Endpoint string `yaml:"endpoint" mapstructure:"endpoint"`
	Path     string `yaml:"path" mapstructure:"path"`
	Type     string `yaml:"type" mapstructure:"type"`
}
