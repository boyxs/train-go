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

type LLMConfig struct {
	Providers []LLMProviderConfig
}

type LLMProviderConfig struct {
	Name      string
	ApiKey    string
	BaseUrl   string
	Model     string
	MaxTokens int
	Timeout   int // 秒
}

type EmbeddingConfig struct {
	BaseUrl string
	ApiKey  string
	Model   string
	Dims    int
	Timeout int // 秒
}
