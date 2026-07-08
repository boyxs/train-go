package viperx

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"
	"gopkg.in/yaml.v3"
)

// EtcdConfig 是 viper 远程配置中心(etcd)的连接信息,从本地 yaml 的 etcd 段读出。
type EtcdConfig struct {
	Endpoints []string `mapstructure:"endpoints"`
	Path      string   `mapstructure:"path"`
	Type      string   `mapstructure:"type"`
}

// reloadTotal 远程配置 reload 次数(status=success/error)。
// Grafana 据 error 增长告警(deploy/grafana/provisioning/alerting/webook-config.yml)。
var reloadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "webook",
	Subsystem: "config",
	Name:      "reload_total",
	Help:      "远程配置 reload 次数(status=success/error)",
}, []string{"status"})

// envPattern 只匹配 ${NAME} 形式(裸 $FOO / 孤立 $ 不匹配),防误伤 dsn 等含 $ 的普通值。
var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvValue 展开字符串里的 ${NAME}(未设置→空);单遍,不二次展开。
func expandEnvValue(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(envPattern.FindStringSubmatch(m)[1])
	})
}

// expandTree 递归展开配置树所有字符串叶子的 ${NAME}(含 slice 元素),原地改写。
// 解析后展开(非解析前替换字节):注入值不经 yaml 解析器,密钥含 #/换行/引号等也无法破坏结构。
func expandTree(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = expandTree(val)
		}
	case []any:
		for i, val := range x {
			x[i] = expandTree(val)
		}
	case string:
		return expandEnvValue(x)
	}
	return v
}

// loadDotEnv 用 godotenv 读 .env 注入进程环境(仅本地便利);不覆盖已有环境变量,缺文件返回 nil。
func loadDotEnv(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return godotenv.Load(path)
}

// LoadLocal 加载本地 yaml:先读配置同目录的 .env 注入 ${ENV} 密钥,再解析 yaml、
// 展开字符串值的 ${NAME}、MergeConfigMap 塞回。--env / APP_ENV 指定路径,缺省 config/local.yaml。
func LoadLocal() error {
	env := pflag.String("env", os.Getenv("APP_ENV"), "运行环境配置文件路径")
	pflag.Parse()
	if *env == "" {
		panic(errors.New("APP_ENV/--env 未设置：容器/部署由 deploy/.env.<env> 注入；本地开发用 APP_ENV=config/local.yaml"))
	}
	// 读配置同目录的 .env 注入密钥(仅本地便利;真实环境变量优先,缺文件跳过)
	if err := loadDotEnv(filepath.Join(filepath.Dir(*env), ".env")); err != nil {
		return err
	}
	raw, err := os.ReadFile(*env)
	if err != nil {
		return err
	}
	// 解析后在字符串值上展开(见 expandTree),塞回 viper
	var tree map[string]any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return err
	}
	if tree != nil {
		expandTree(tree)
		if err := viper.MergeConfigMap(tree); err != nil {
			return err
		}
	}
	log.Printf("[config] loaded: %s", *env)
	return nil
}

// WatchRemote 叠加 etcd 远程配置 + 起 5s 轮询 watch,reload 成功调 onChange。远程值逐键 viper.Set
// 进 override 层(绕开 viper file>kvstore 的本地优先);endpoints 空/不可达/解码失败只告警沿用本地,不阻断。
func WatchRemote(cfg EtcdConfig, onChange func()) {
	if len(cfg.Endpoints) == 0 {
		log.Println("[config] etcd.endpoints 未配置,跳过远程配置中心,仅用本地 yaml")
		return
	}
	rv := viper.New()
	for _, ep := range cfg.Endpoints {
		if err := rv.AddRemoteProvider("etcd3", ep, cfg.Path); err != nil {
			log.Printf("[config] 注册 etcd provider 失败,使用本地配置: %v", err)
			return
		}
	}
	rv.SetConfigType(cfg.Type)
	if err := rv.ReadRemoteConfig(); err != nil {
		reloadTotal.WithLabelValues("error").Inc()
		log.Printf("[config] 远程配置加载失败,使用本地配置: %v", err)
		return
	}
	applyRemote(rv)
	reloadTotal.WithLabelValues("success").Inc()
	log.Printf("[config] 远程配置加载成功: endpoints=%v key=%s", cfg.Endpoints, cfg.Path)
	go watchLoop(rv, onChange)
}

// applyRemote 把远程子集逐键写进全局 viper 的 override 层(最高优先级),确保远程值覆盖本地默认。
func applyRemote(rv *viper.Viper) {
	for _, k := range rv.AllKeys() {
		viper.Set(k, rv.Get(k))
	}
}

// watchLoop 5s 轮询 etcd,reload 成功后写 override 层并触发 onChange(配置热更回调)。
func watchLoop(rv *viper.Viper, onChange func()) {
	for {
		time.Sleep(5 * time.Second)
		if err := rv.ReadRemoteConfig(); err != nil {
			reloadTotal.WithLabelValues("error").Inc()
			log.Printf("[config] watch 失败: %v", err)
			continue
		}
		applyRemote(rv)
		reloadTotal.WithLabelValues("success").Inc()
		if onChange != nil {
			onChange()
		}
	}
}
