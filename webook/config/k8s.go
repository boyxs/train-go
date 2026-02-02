//go:build !k8s

package config

var Config = config{
	MySQL: MySQLConfig{
		DSN: "root:13520@tcp(localhost:3306)/webook",
	},
	Redis: RedisConfig{
		Addr:     "127.0.0.1:6379",
		Password: "13520",
	},
}
