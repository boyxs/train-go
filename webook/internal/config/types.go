package config

type config struct {
	MySQL MySQLConfig
	Redis RedisConfig
}

type MySQLConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr     string
	Password string
}
