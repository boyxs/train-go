package web

import (
	"errors"
	"net/http"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/service"
	"github.com/webook/internal/web/jwt"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	jwt2 "github.com/golang-jwt/jwt/v5"

	//"regexp" 此库不支持 (?=
	regexp "github.com/dlclark/regexp2"
)

const (
	emailExpr = "^\\w+([-+.]\\w+)*@\\w+([-.]\\w+)*\\.\\w+([-.]\\w+)*$"
	// 和上面比起来，用 ` 看起来就比较清爽
	passwordExpr = `^(?=.*[A-Za-z])(?=.*\d)(?=.*[$@$!%*#?&])[A-Za-z\d$@$!%*#?&]{8,}$`
	loginBiz     = "login"
)

type UserHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalUserHandler struct {
	jwt.JwtHandler
	emailRegexp    *regexp.Regexp
	passwordRegexp *regexp.Regexp
	userService    service.UserService
	codeService    service.CodeService
	l              logger.LoggerX
}

type UserClaims = jwt.UserClaims

func NewInternalUserHandler(hdl jwt.JwtHandler, us service.UserService, cs service.CodeService, l logger.LoggerX) UserHandler {
	er := regexp.MustCompile(emailExpr, regexp.None)
	pr := regexp.MustCompile(passwordExpr, regexp.None)
	return &InternalUserHandler{
		JwtHandler:     hdl,
		emailRegexp:    er,
		passwordRegexp: pr,
		userService:    us,
		codeService:    cs,
		l:              l,
	}
}

type registerReq struct {
	Email           string `json:"email" binding:"required"`
	Password        string `json:"password" binding:"required"`
	ConfirmPassword string `json:"confirmPassword" binding:"required"`
}

type smsCodeReq struct {
	Phone string `json:"phone"`
}

type loginSMSReq struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type editReqUser struct {
	Nickname string `json:"nickname"`
	Birthday int64  `json:"birthday"`
	AboutMe  string `json:"aboutMe"`
}

func (h *InternalUserHandler) RegisterRoutes(server *gin.Engine) {
	ug := server.Group("/user")
	ug.POST("/register", ginx.WrapReq[registerReq](h.Register))
	ug.POST("/login", ginx.WrapReq[loginReq](h.Login))
	ug.POST("/logout", ginx.Wrap(h.Logout))
	ug.POST("/edit", ginx.WrapReqClaims[editReqUser, UserClaims](consts.UserKey, h.Edit))
	ug.GET("/refresh_token", h.RefreshToken) // 直接返 401，不走 wrapper
	ug.GET("/profile", h.Profile) // 不走 wrapper：前端期望直接 Profile 不带 Result 包装

	//手机验证码登录相关功能
	ug.POST("/login_sms/code/send", ginx.WrapReq[smsCodeReq](h.SendLoginSMSCode))
	ug.POST("/login_sms", ginx.WrapReq[loginSMSReq](h.LoginSMS))
}

func (h *InternalUserHandler) Register(ctx *gin.Context, req registerReq) (ginx.Result, error) {
	isEmail, err := h.emailRegexp.MatchString(req.Email)
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	if !isEmail {
		return ginx.Result{Code: 4, Msg: "非法邮箱格式"}, nil
	}
	if req.Password != req.ConfirmPassword {
		return ginx.Result{Code: 4, Msg: "两次输入密码不匹配"}, nil
	}
	isPassword, err := h.passwordRegexp.MatchString(req.Password)
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	if !isPassword {
		return ginx.Result{Code: 4, Msg: "密码必须包含字母、数字、特殊字符，并且不少于八位"}, nil
	}
	err = h.userService.Register(ctx, domain.User{
		Email:    req.Email,
		Password: req.Password,
	})
	if errors.Is(err, service.ErrDuplicateEmail) {
		return ginx.Result{Code: 4, Msg: "邮箱已被注册"}, nil
	}
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统异常"}, err
	}
	return ginx.Result{Msg: "注册成功"}, nil
}

func (h *InternalUserHandler) SendLoginSMSCode(ctx *gin.Context, req smsCodeReq) (ginx.Result, error) {
	if req.Phone == "" {
		return ginx.Result{Code: 4, Msg: "请输入手机号码"}, nil
	}
	err := h.codeService.Send(ctx, loginBiz, req.Phone)
	switch {
	case err == nil:
		return ginx.Result{Msg: "发送成功"}, nil
	case errors.Is(err, service.ErrCodeSendTooMany):
		return ginx.Result{Code: 4, Msg: "短信发送太频繁，请稍后再试"}, nil
	default:
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
}

func (h *InternalUserHandler) LoginSMS(ctx *gin.Context, req loginSMSReq) (ginx.Result, error) {
	ok, err := h.codeService.Verify(ctx, loginBiz, req.Phone, req.Code)
	if err != nil {
		return ginx.Result{Code: 5, Msg: err.Error()}, err
	}
	if !ok {
		return ginx.Result{Code: 4, Msg: "验证码错误，请重新输入"}, nil
	}
	user, err := h.userService.FindOrCreate(ctx, req.Phone)
	if err != nil {
		return ginx.Result{Code: 5, Msg: err.Error()}, err
	}
	if err := h.SetLoginToken(ctx, user.Id); err != nil {
		return ginx.Result{Code: 5, Msg: "系统异常"}, err
	}
	return ginx.Result{Msg: "登录成功"}, nil
}

func (h *InternalUserHandler) Login(ctx *gin.Context, req loginReq) (ginx.Result, error) {
	user, err := h.userService.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:
		if err := h.SetLoginToken(ctx, user.Id); err != nil {
			return ginx.Result{Code: 5, Msg: "系统异常"}, err
		}
		return ginx.Result{Msg: "登录成功"}, nil
	case service.ErrInvalidUserOrPassword:
		return ginx.Result{Code: 4, Msg: "用户名或密码错误"}, nil
	default:
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
}

func (h *InternalUserHandler) Logout(ctx *gin.Context) (ginx.Result, error) {
	if err := h.ClearToken(ctx); err != nil {
		return ginx.Result{Code: 5, Msg: "退出失败"}, err
	}
	return ginx.Result{Msg: "退出成功"}, nil
}

// RefreshToken 不走 wrapper：失败直接 401 让前端跳登录
func (h *InternalUserHandler) RefreshToken(ctx *gin.Context) {
	tokenStr := h.ExtractToken(ctx)
	var rc jwt.RefreshClaims
	token, err := jwt2.ParseWithClaims(tokenStr, &rc, func(token *jwt2.Token) (any, error) {
		return consts.RefreshKey, nil
	})
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if token == nil || !token.Valid {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.CheckSession(ctx, rc.Ssid); err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.SetAccessToken(ctx, rc.Userid, rc.Ssid); err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	ctx.JSON(http.StatusOK, ginx.Result{Msg: "OK"})
}

func (h *InternalUserHandler) Edit(ctx *gin.Context, req editReqUser, uc UserClaims) (ginx.Result, error) {
	user, err := h.userService.Edit(ctx, domain.User{
		Id:       uc.Userid,
		Nickname: req.Nickname,
		Birthday: req.Birthday,
		AboutMe:  req.AboutMe,
	})
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	return ginx.Result{Data: user}, nil
}

// Profile 不走 wrapper：保持原格式直接返回 Profile 对象（前端依赖此格式）
func (h *InternalUserHandler) Profile(ctx *gin.Context) {
	uc, ok := ctx.MustGet(consts.UserKey).(UserClaims)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, "请先登录")
		return
	}
	profile, err := h.userService.Profile(ctx, uc.Userid)
	if err != nil {
		ctx.JSON(http.StatusOK, ginx.Result{Code: 5, Msg: "系统异常"})
		return
	}
	ctx.JSON(http.StatusOK, profile)
}

// ==================Session Operations==================

func (h *InternalUserHandler) LoginSS(ctx *gin.Context, req loginReq) (ginx.Result, error) {
	user, err := h.userService.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:
		session := sessions.Default(ctx)
		session.Set("userid", user.Id)
		session.Options(sessions.Options{
			Path:     "/",
			MaxAge:   int(consts.ExpireTime.Minutes()),
			Secure:   true,
			HttpOnly: true,
		})
		if err := session.Save(); err != nil {
			return ginx.Result{Code: 5, Msg: "系统异常"}, err
		}
		return ginx.Result{Msg: "登录成功"}, nil
	case service.ErrInvalidUserOrPassword:
		return ginx.Result{Code: 4, Msg: "用户名或密码错误"}, nil
	default:
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
}

func (h *InternalUserHandler) LogoutSS(ctx *gin.Context) (ginx.Result, error) {
	session := sessions.Default(ctx)
	session.Delete("userid")
	if err := session.Save(); err != nil {
		return ginx.Result{Code: 5, Msg: "退出失败"}, err
	}
	return ginx.Result{Msg: "退出成功"}, nil
}

func (h *InternalUserHandler) EditSS(ctx *gin.Context, req editReqUser) (ginx.Result, error) {
	session := sessions.Default(ctx)
	userid := session.Get("userid")
	user, err := h.userService.Edit(ctx, domain.User{
		Id:       userid.(int64),
		Nickname: req.Nickname,
		Birthday: req.Birthday,
		AboutMe:  req.AboutMe,
	})
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统错误"}, err
	}
	return ginx.Result{Data: user}, nil
}

func (h *InternalUserHandler) ProfileSS(ctx *gin.Context) (ginx.Result, error) {
	session := sessions.Default(ctx)
	val := session.Get("userid")
	userid, ok := val.(int64)
	if !ok {
		return ginx.Result{Code: 4, Msg: "请先登录"}, nil
	}
	profile, err := h.userService.Profile(ctx, userid)
	if err != nil {
		return ginx.Result{Code: 5, Msg: "系统异常"}, err
	}
	return ginx.Result{Data: profile}, nil
}
