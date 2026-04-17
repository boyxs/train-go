package jwt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/webook/internal/consts"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrTokenInvalid = errors.New("token invalid")

type RedisJwtHandler struct {
	cmd           redis.Cmdable
	signingMethod jwt.SigningMethod
	expiration    time.Duration
}

func NewRedisJwtHandler(cmd redis.Cmdable) JwtHandler {
	return &RedisJwtHandler{
		cmd:           cmd,
		signingMethod: jwt.SigningMethodHS512,
		expiration:    consts.ExpireTime,
	}
}

func (h *RedisJwtHandler) SetLoginToken(ctx *gin.Context, userid int64) error {
	ssid := uuid.New().String()
	err := h.SetRefreshToken(ctx, userid, ssid)
	if err != nil {
		return err
	}
	return h.SetAccessToken(ctx, userid, ssid)
}

func (h *RedisJwtHandler) SetAccessToken(ctx *gin.Context, userid int64, ssid string) error {
	uc := UserClaims{
		Userid:    userid,
		Ssid:      ssid,
		UserAgent: ctx.GetHeader("User-Agent"),
		RegisteredClaims: jwt.RegisteredClaims{
			// 30 分钟过期
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(consts.ExpireTime)),
		},
	}
	token := jwt.NewWithClaims(h.signingMethod, uc)
	tokenStr, err := token.SignedString(consts.AccessKey)
	if err != nil {
		return err
	}
	ctx.Header(consts.AccessHeader, tokenStr)
	return nil
}

func (h *RedisJwtHandler) SetRefreshToken(ctx *gin.Context, userid int64, ssid string) error {
	rc := RefreshClaims{
		Userid: userid,
		Ssid:   ssid,
		RegisteredClaims: jwt.RegisteredClaims{
			// 7 天过期
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(consts.RefreshTime)),
		},
	}
	token := jwt.NewWithClaims(h.signingMethod, rc)
	tokenStr, err := token.SignedString(consts.RefreshKey)
	if err != nil {
		return err
	}
	ctx.Header(consts.RefreshHeader, tokenStr)
	return nil
}

func (h *RedisJwtHandler) ExtractToken(ctx *gin.Context) string {
	authorization := ctx.GetHeader(consts.Authorization)
	if authorization == "" {
		return authorization
	}
	prefix, token, found := strings.Cut(authorization, " ")
	if !found || !strings.EqualFold(prefix, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func (h *RedisJwtHandler) CheckSession(ctx *gin.Context, ssid string) error {
	cnt, err := h.cmd.Exists(ctx, fmt.Sprintf(consts.UserSsidPattern, ssid)).Result()
	if err != nil {
		// Redis 不可用时容错放行，宁可放行不可误判已登录用户为未登录
		return nil
	}
	if cnt > 0 {
		return ErrTokenInvalid
	}
	return nil
}

func (h *RedisJwtHandler) ClearToken(ctx *gin.Context) error {
	ctx.Header(consts.AccessHeader, "")
	ctx.Header(consts.RefreshHeader, "")
	uc := ctx.MustGet(consts.UserKey).(UserClaims)
	//设置ssid强制让token失效，过期时间必须大于等于token有效时间
	return h.cmd.Set(ctx, fmt.Sprintf(consts.UserSsidPattern, uc.Ssid), "", h.expiration).Err()
}

type UserClaims struct {
	jwt.RegisteredClaims
	Userid    int64
	Ssid      string
	UserAgent string
}

type RefreshClaims struct {
	jwt.RegisteredClaims
	Userid int64
	Ssid   string
}
