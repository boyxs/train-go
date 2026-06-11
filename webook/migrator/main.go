package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"

	"github.com/webook/migrator/ioc"
)

func main() {
	initViper()
	app, cleanup, err := InitApp()
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// http.addr 由 yaml 提供；fallback 仅在漏配时兜底（与 chat :8189、core :8081 错开）
	addr := viper.GetString("http.addr")
	if addr == "" {
		addr = ":8083"
	}
	log.Printf("[migrator] listening on %s", addr)
	if err := app.Server.Run(addr); err != nil {
		log.Fatalf("[migrator] exit: %v", err)
	}
}

// initViper 与 chat/main.go initViper 同构：本地 yaml + etcd 远程叠加。
func initViper() {
	env := pflag.String("env", os.Getenv("APP_ENV"), "运行环境配置文件路径")
	pflag.Parse()
	if *env == "" {
		*env = "config/local.yaml"
	}
	viper.SetConfigFile(*env)
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
	// 环境变量覆盖 yaml（K8s envFrom.secretRef 注入 Secret 时生效）：
	// 例 MYSQL_DSN 覆盖 viper.GetString("mysql.dsn")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	log.Printf("[migrator] config loaded: %s", viper.ConfigFileUsed())

	cfg := EtcdConfig{
		Endpoint: "http://127.0.0.1:2379",
		Path:     "/webook-migrator",
		Type:     "yaml",
	}
	if err := viper.UnmarshalKey("etcd", &cfg); err != nil {
		panic(err)
	}
	initViperRemote(cfg)
}

// initViperRemote 拉取 etcd 远程配置 + 启动后台 watch（5s 轮询）。
// 远程不可达 / 解码失败：日志告警，沿用本地 yaml，不 panic（避免 etcd 故障让所有服务起不来）。
func initViperRemote(cfg EtcdConfig) {
	if err := viper.AddRemoteProvider("etcd3", cfg.Endpoint, cfg.Path); err != nil {
		log.Printf("[migrator] 注册 etcd provider 失败，使用本地配置: %v", err)
		return
	}
	viper.SetConfigType(cfg.Type)
	if err := viper.ReadRemoteConfig(); err != nil {
		log.Printf("[migrator] 远程配置加载失败，使用本地配置: %v", err)
		return
	}
	log.Printf("[migrator] 远程配置加载成功: endpoint=%s key=%s", cfg.Endpoint, cfg.Path)
	go func() {
		for {
			time.Sleep(5 * time.Second)
			if err := viper.ReadRemoteConfig(); err != nil {
				log.Printf("[migrator] watch 失败: %v", err)
				continue
			}
			for _, fn := range ioc.ConfigChangeCallbacks {
				fn()
			}
		}
	}()
}

// EtcdConfig 与 chat/main.go 字段同源。
type EtcdConfig struct {
	Endpoint string `yaml:"endpoint" mapstructure:"endpoint"`
	Path     string `yaml:"path" mapstructure:"path"`
	Type     string `yaml:"type" mapstructure:"type"`
}
