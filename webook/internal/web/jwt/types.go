package jwt

import "github.com/gin-gonic/gin"

type JwtHandler interface {
	SetLoginToken(ctx *gin.Context, userid int64) error
	SetAccessToken(ctx *gin.Context, userid int64, ssid string) error
	SetRefreshToken(ctx *gin.Context, userid int64, ssid string) error
	ExtractToken(ctx *gin.Context) string
	CheckSession(ctx *gin.Context, ssid string) error
	ClearToken(ctx *gin.Context) error
}
