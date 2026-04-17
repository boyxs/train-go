package consts

import "time"

// JWT 认证相关
var (
	AccessKey         = []byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK")
	RefreshKey        = []byte("k6CswdUm77WKcbM68UQUuxVsHSpTCwgA")
	AccessHeader      = "x-access-token"
	RefreshHeader     = "x-refresh-token"
	Authorization     = "Authorization"
	UserAgent         = "User-Agent"
	UserKey           = "user_claims"
	Interval          = time.Second * 10
	ExpireTime        = time.Minute * 30
	RefreshTime       = time.Hour * 24 * 7
	RefreshThreshold  = ExpireTime - Interval
	LastEventIDHeader = "Last-Event-ID"
)
