package web

import (
	"fmt"
	"net/http"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/service/oauth2"
	myJwt "gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type OAuth2Handler interface {
	RegisterRoutes(server *gin.Engine)
	AuthURL(ctx *gin.Context)
	Callback(ctx *gin.Context)
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
	og.GET("/authurl", h.AuthURL)
	og.Any("/callback", h.Callback)
}

func (h *OAuth2WechatHandler) AuthURL(ctx *gin.Context) {
	state := uuid.New().String()
	authURL, err := h.svc.AuthURL(ctx, state)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "构造授权登录URL失败",
		})
		return
	}
	err = h.setStateCookie(ctx, state)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg:  "服务器异常",
			Code: 5,
		})
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Data: authURL,
	})
}

func (h *OAuth2WechatHandler) Callback(ctx *gin.Context) {
	err := h.verifyState(ctx)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg:  "非法请求",
			Code: 4,
		})
		return
	}
	//可选校验
	code := ctx.Query("code")
	wechatAuth, err := h.svc.VerifyCode(ctx, code)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg:  "授权码有误",
			Code: 4,
		})
		return
	}
	u, err := h.userSvc.FindOrCreateByWechat(ctx, wechatAuth)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Msg:  "系统错误",
			Code: 5,
		})
		return
	}
	err = h.SetLoginToken(ctx, u.Id)
	if err != nil {
		ctx.String(http.StatusOK, "系统错误")
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "OK",
	})
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
		// state 不匹配，有人搞鬼
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
