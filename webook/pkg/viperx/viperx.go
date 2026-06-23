package viperx

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"
)

// EtcdConfig 是 viper 远程配置中心(etcd)的连接信息,从本地 yaml 的 etcd 段读出。
type EtcdConfig struct {
	Endpoints []string `mapstructure:"endpoints"`
	Path      string   `mapstructure:"path"`
	Type      string   `mapstructure:"type"`
}

// LoadLocal 加载本地 yaml:--env 指定路径(缺省取 APP_ENV,再缺省 config/local.yaml),
// 并开启环境变量覆盖(K8s 用 envFrom.secretRef 注入 Secret 时生效)。
func LoadLocal() error {
	env := pflag.String("env", os.Getenv("APP_ENV"), "运行环境配置文件路径")
	pflag.Parse()
	if *env == "" {
		*env = "config/local.yaml"
	}
	viper.SetConfigFile(*env)
	if err := viper.ReadInConfig(); err != nil {
		return err
	}
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // 把 http.addr 映射成 HTTP_ADDR
	log.Printf("[config] loaded: %s", viper.ConfigFileUsed())
	return nil
}

// WatchRemote 叠加 etcd 远程配置并起后台 watch(5s 轮询),每次成功 reload 后调 onChange。
// endpoints 未配置 / 远程不可达 / 解码失败均只告警、沿用本地 yaml,不阻断启动。
func WatchRemote(cfg EtcdConfig, onChange func()) {
	if len(cfg.Endpoints) == 0 {
		log.Println("[config] etcd.endpoints 未配置,跳过远程配置中心,仅用本地 yaml")
		return
	}
	for _, ep := range cfg.Endpoints {
		if err := viper.AddRemoteProvider("etcd3", ep, cfg.Path); err != nil {
			log.Printf("[config] 注册 etcd provider 失败,使用本地配置: %v", err)
			return
		}
	}
	viper.SetConfigType(cfg.Type)
	if err := viper.ReadRemoteConfig(); err != nil {
		log.Printf("[config] 远程配置加载失败,使用本地配置: %v", err)
		return
	}
	log.Printf("[config] 远程配置加载成功: endpoints=%v key=%s", cfg.Endpoints, cfg.Path)
	go watchLoop(onChange)
}

// watchLoop 5s 轮询 etcd,reload 成功后触发 onChange(配置热更回调)。
func watchLoop(onChange func()) {
	for {
		time.Sleep(5 * time.Second)
		if err := viper.ReadRemoteConfig(); err != nil {
			log.Printf("[config] watch 失败: %v", err)
			continue
		}
		if onChange != nil {
			onChange()
		}
	}
}
