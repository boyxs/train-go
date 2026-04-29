package consts

// JWT 验签相关常量。chat 服务只验签不签发，密钥与主仓 internal/consts/auth.go 同源。
// access/refresh token 不签发，所以 RefreshKey、Authorization、UserAgent 等签发相关常量不需要。
var (
	AccessKey         = []byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK")
	AccessHeader      = "x-access-token"  // CORS AllowHeaders 用，服务端不会写回（chat 不刷新 token）
	RefreshHeader     = "x-refresh-token" // CORS AllowHeaders 用，同上
	UserKey           = "user_claims"
	LastEventIDHeader = "Last-Event-ID"
	UserSsidPattern   = "user:ssid:%s" // 与主仓 internal/consts/cache.go 同源
)
