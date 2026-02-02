//go:build k8s

package config

var Config = config{
	MySQL: MySQLConfig{
		DSN: "root:13520@tcp(webook-record-mysql:3306)/webook",
	},
	Redis: RedisConfig{
		Addr:     "webook-record-redis:6379",
		Password: "13520",
	},
}
