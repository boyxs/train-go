package consts

// JWT 验签相关常量。
//
// migrator 服务只验签不签发（控制台 token 由 webook-core 颁发），与
// chat/consts/auth.go、internal/consts/auth.go 同源（同一密钥 = 同一用户体系）。
var (
	AccessKey       = []byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK")
	AccessHeader    = "x-access-token"
	RefreshHeader   = "x-refresh-token"
	UserKey         = "user_claims"
	UserSsidPattern = "user:ssid:%s"
)
