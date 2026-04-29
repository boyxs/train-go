package ioc

import (
	"github.com/redis/go-redis/v9"

	"github.com/webook/internal/consts"
	"github.com/webook/pkg/jwtx"
)

// InitJwtHandler 装配 pkg/jwtx 的 JWT 处理器（签发 + 提取 + 校验 + 登出全套）。
// 密钥 / 头名 / TTL 等业务参数从 internal/consts 取，传入 pkg 完成解耦。
func InitJwtHandler(cmd redis.Cmdable) jwtx.Handler {
	return jwtx.NewRedisHandler(cmd, jwtx.HandlerConfig{
		AccessKey:      consts.AccessKey,
		RefreshKey:     consts.RefreshKey,
		AccessHeader:   consts.AccessHeader,
		RefreshHeader:  consts.RefreshHeader,
		AccessExpire:   consts.ExpireTime,
		RefreshExpire:  consts.RefreshTime,
		SsidKeyPattern: consts.UserSsidPattern,
		UserKey:        consts.UserKey,
	})
}
