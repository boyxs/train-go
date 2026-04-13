package web

import (
	"fmt"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/oauth2"
	myJwt "gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ginx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type OAuth2Handler interface {
	RegisterRoutes(server *gin.Engine)
}

type OAuth2WechatHandler struct {
	myJwt.JwtHandler
	svc             oauth2.OAuth2Service
	userSvc         service.UserService
	key             []byte
	stateCookieName string
}

func NewOAuth2WechatHandler(
	hdl myJwt.JwtHandler,
	svc oauth2.OAuth2Service,
	userSvc service.UserService,
) OAuth2Handler {
	return &OAuth2WechatHandler{
		JwtHandler:      hdl,
		svc:             svc,
		userSvc:         userSvc,
		key:             consts.WechatKey,
		stateCookieName: consts.StateCookieName,
	}
}

func (h *OAuth2WechatHandler) RegisterRoutes(server *gin.Engine) {
	og := server.Group("/oauth2/wechat")
	og.GET("/authurl", ginx.Wrap(h.AuthURL))
	og.Any("/callback", ginx.Wrap(h.Callback))
}

func (h *OAuth2WechatHandler) AuthURL(ctx *gin.Context) (ginx.Result, error) {
	state := uuid.New().String()
	authURL, err := h.svc.AuthURL(ctx, state)
	if err != nil {
		return ginx.Result{Code: 5, Msg: "构造授权登录URL失败"}, err
	}
	if err := h.setStateCookie(ctx, state); err != nil {
		return ginx.Result{Code: 5, Msg: "服务器异常"}, err
	}
	return ginx.Result{Data: authURL}, nil
}

func (h *OAuth2WechatHandler) Callback(ctx *gin.Context) (ginx.Result, error) {
	if err := h.verifyState(ctx); err != nil {
		return ginx.Result{Code: 4, Msg: "非法请求"}, nil
	}
	code := ctx.Query("code")
	wechatAuth, err := h.svc.VerifyCode(ctx, code)
	if err != nil {
		return ginx.Result{Code: 4, Msg: "授权码有误"}, nil
	}
	u, err := h.userSvc.FindOrCreateByWechat(ctx, wechatAuth)
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	if err := h.SetLoginToken(ctx, u.Id); err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *OAuth2WechatHandler) verifyState(ctx *gin.Context) error {
	state := ctx.Query("state")
	ck, err := ctx.Cookie(h.stateCookieName)
	if err != nil {
		return fmt.Errorf("无法获得 cookie %w", err)
	}
	var sc StateClaims
	_, err = jwt.ParseWithClaims(ck, &sc, func(token *jwt.Token) (any, error) {
		return h.key, nil
	})
	if err != nil {
		return fmt.Errorf("解析 token 失败 %w", err)
	}
	if state != sc.State {
		return fmt.Errorf("state 不匹配")
	}
	return nil
}

func (h *OAuth2WechatHandler) setStateCookie(ctx *gin.Context, state string) error {
	sc := StateClaims{
		State: state,
	}
	token := jwt.NewWithClaims(consts.SigningMethod, sc)
	tokenStr, err := token.SignedString(h.key)
	if err != nil {
		return err
	}
	ctx.SetCookie(h.stateCookieName, tokenStr,
		int((10 * time.Minute).Seconds()), "/oauth2/wechat/callback",
		"", false, true)
	return nil
}

type StateClaims struct {
	jwt.RegisteredClaims
	State string
}
