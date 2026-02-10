//go:build k8s

package config

var Config = config{
	MySQL: MySQLConfig{
		DSN: "root:13520@tcp(webook-mysql:3307)/webook",
	},
	Redis: RedisConfig{
		Addr:     "webook-redis:6380",
		Password: "13520",
	},
}
