package jwtx

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	stdjwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisHandler JWT 处理器：签发 access/refresh、提取、登出黑名单走 Redis。
// 跨服务复用：webook-core 登录用全套；chat 不需要（只走 MiddlewareBuilder 验签）。
type RedisHandler struct {
	cmd           redis.Cmdable
	signingMethod stdjwt.SigningMethod
	cfg           HandlerConfig
}

func NewRedisHandler(cmd redis.Cmdable, cfg HandlerConfig) Handler {
	return &RedisHandler{
		cmd:           cmd,
		signingMethod: stdjwt.SigningMethodHS512,
		cfg:           cfg,
	}
}

func (h *RedisHandler) SetLoginToken(ctx *gin.Context, userid int64) error {
	ssid := uuid.New().String()
	if err := h.SetRefreshToken(ctx, userid, ssid); err != nil {
		return err
	}
	return h.SetAccessToken(ctx, userid, ssid)
}

func (h *RedisHandler) SetAccessToken(ctx *gin.Context, userid int64, ssid string) error {
	uc := UserClaims{
		Userid:    userid,
		Ssid:      ssid,
		UserAgent: ctx.GetHeader(HeaderUserAgent),
		RegisteredClaims: stdjwt.RegisteredClaims{
			ExpiresAt: stdjwt.NewNumericDate(time.Now().Add(h.cfg.AccessExpire)),
		},
	}
	token := stdjwt.NewWithClaims(h.signingMethod, uc)
	tokenStr, err := token.SignedString(h.cfg.AccessKey)
	if err != nil {
		return err
	}
	ctx.Header(h.cfg.AccessHeader, tokenStr)
	return nil
}

func (h *RedisHandler) SetRefreshToken(ctx *gin.Context, userid int64, ssid string) error {
	rc := RefreshClaims{
		Userid: userid,
		Ssid:   ssid,
		RegisteredClaims: stdjwt.RegisteredClaims{
			ExpiresAt: stdjwt.NewNumericDate(time.Now().Add(h.cfg.RefreshExpire)),
		},
	}
	token := stdjwt.NewWithClaims(h.signingMethod, rc)
	tokenStr, err := token.SignedString(h.cfg.RefreshKey)
	if err != nil {
		return err
	}
	ctx.Header(h.cfg.RefreshHeader, tokenStr)
	return nil
}

// ExtractToken 等价 ExtractBearer，保留方法是为了实现 Handler 接口。
func (h *RedisHandler) ExtractToken(ctx *gin.Context) string {
	return ExtractBearer(ctx)
}

func (h *RedisHandler) CheckSession(ctx *gin.Context, ssid string) error {
	cnt, err := h.cmd.Exists(ctx, fmt.Sprintf(h.cfg.SsidKeyPattern, ssid)).Result()
	if err != nil {
		// Redis 不可用时容错放行，宁可放行不可误判已登录用户为未登录
		return nil
	}
	if cnt > 0 {
		return ErrTokenInvalid
	}
	return nil
}

func (h *RedisHandler) ClearToken(ctx *gin.Context) error {
	ctx.Header(h.cfg.AccessHeader, "")
	ctx.Header(h.cfg.RefreshHeader, "")
	uc := ctx.MustGet(h.cfg.UserKey).(UserClaims)
	// 设置 ssid 强制 token 失效，过期时间 = access token 过期，足够覆盖未消费的 access token
	return h.cmd.Set(ctx, fmt.Sprintf(h.cfg.SsidKeyPattern, uc.Ssid), "", h.cfg.AccessExpire).Err()
}
