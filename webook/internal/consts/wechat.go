package consts

import "github.com/golang-jwt/jwt/v5"

var (
	WechatKey       = []byte("k6CswdUm77WKcbM68UQUuxVsHSpTCwgB")
	StateCookieName = "jwt-state-cookie"
	SigningMethod   = jwt.SigningMethodHS512
)
