package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	jwt2 "github.com/golang-jwt/jwt/v5"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/errs"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	jwt "github.com/webook/pkg/jwtx"
	"github.com/webook/pkg/logger"

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
	JwtHandler     jwt.Handler
	emailRegexp    *regexp.Regexp
	passwordRegexp *regexp.Regexp
	userService    service.UserService
	codeService    service.CodeService
	l              logger.LoggerX
}

type UserClaims = jwt.UserClaims

func NewInternalUserHandler(hdl jwt.Handler, us service.UserService, cs service.CodeService, l logger.LoggerX) UserHandler {
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

// userInfoReq 他人主页取某用户公开信息
type userInfoReq struct {
	Id int64 `json:"id" binding:"required,gt=0"`
}

// userInfoVO 他人主页头部用户公开信息（不含邮箱/手机等私密字段）
type userInfoVO struct {
	Id        int64  `json:"id"`
	Nickname  string `json:"nickname"`
	AboutMe   string `json:"aboutMe"`
	CreatedAt int64  `json:"createdAt"`
}

func (h *InternalUserHandler) RegisterRoutes(server *gin.Engine) {
	ug := server.Group("/user")
	ug.POST("/register", ginx.WrapReq[registerReq](h.Register))
	ug.POST("/login", ginx.WrapReq[loginReq](h.Login))
	ug.POST("/logout", ginx.Wrap(h.Logout))
	ug.POST("/edit", ginx.WrapReq[editReqUser](h.Edit))
	ug.GET("/refresh_token", h.RefreshToken)            // 直接返 401，不走 wrapper
	ug.GET("/profile", h.Profile)                       // 不走 wrapper：前端期望直接 Profile 不带 Result 包装
	ug.POST("/info", ginx.WrapReq[userInfoReq](h.Info)) // 公开：他人主页取昵称/简介/加入时间

	//手机验证码登录相关功能
	ug.POST("/login_sms/code/send", ginx.WrapReq[smsCodeReq](h.SendLoginSMSCode))
	ug.POST("/login_sms", ginx.WrapReq[loginSMSReq](h.LoginSMS))
}

func (h *InternalUserHandler) Register(ctx *gin.Context, req registerReq) (ginx.Result, error) {
	isEmail, err := h.emailRegexp.MatchString(req.Email)
	if err != nil {
		return ginx.Result{}, err
	}
	if !isEmail {
		return ginx.Result{}, errs.ErrInvalidEmailFormat
	}
	if req.Password != req.ConfirmPassword {
		return ginx.Result{}, errs.ErrPasswordMismatch
	}
	isPassword, err := h.passwordRegexp.MatchString(req.Password)
	if err != nil {
		return ginx.Result{}, err
	}
	if !isPassword {
		return ginx.Result{}, errs.ErrPasswordWeak
	}
	if err := h.userService.Register(ctx, domain.User{
		Email:    req.Email,
		Password: req.Password,
	}); err != nil {
		return ginx.Result{}, err // ErrDuplicateEmail 自带 409，其他系统错误自动 500
	}
	return ginx.Result{Msg: "注册成功"}, nil
}

func (h *InternalUserHandler) SendLoginSMSCode(ctx *gin.Context, req smsCodeReq) (ginx.Result, error) {
	if req.Phone == "" {
		return ginx.Result{}, errs.ErrPhoneEmpty
	}
	if err := h.codeService.Send(ctx, loginBiz, req.Phone); err != nil {
		return ginx.Result{}, err // ErrCodeSendTooMany 自带 429
	}
	return ginx.Result{Msg: "发送成功"}, nil
}

func (h *InternalUserHandler) LoginSMS(ctx *gin.Context, req loginSMSReq) (ginx.Result, error) {
	ok, err := h.codeService.Verify(ctx, loginBiz, req.Phone, req.Code)
	if err != nil {
		return ginx.Result{}, err
	}
	if !ok {
		return ginx.Result{}, errs.ErrSMSCodeWrong
	}
	user, err := h.userService.FindOrCreate(ctx, req.Phone)
	if err != nil {
		return ginx.Result{}, err
	}
	if err := h.JwtHandler.SetLoginToken(ctx, user.Id); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "登录成功"}, nil
}

func (h *InternalUserHandler) Login(ctx *gin.Context, req loginReq) (ginx.Result, error) {
	user, err := h.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		return ginx.Result{}, err // ErrInvalidUserOrPassword 自带 401，其他自动 500
	}
	if err := h.JwtHandler.SetLoginToken(ctx, user.Id); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "登录成功"}, nil
}

func (h *InternalUserHandler) Logout(ctx *gin.Context) (ginx.Result, error) {
	if err := h.JwtHandler.ClearToken(ctx); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "退出成功"}, nil
}

// RefreshToken 不走 wrapper：失败直接 401 让前端跳登录
func (h *InternalUserHandler) RefreshToken(ctx *gin.Context) {
	tokenStr := h.JwtHandler.ExtractToken(ctx)
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
	if err := h.JwtHandler.CheckSession(ctx, rc.Ssid); err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.JwtHandler.SetAccessToken(ctx, rc.Userid, rc.Ssid); err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	ctx.JSON(http.StatusOK, ginx.Result{Msg: "OK"})
}

func (h *InternalUserHandler) Edit(ctx *gin.Context, req editReqUser) (ginx.Result, error) {
	uc := ginx.MustClaims[UserClaims](ctx)
	user, err := h.userService.Edit(ctx, domain.User{
		Id:       uc.Userid,
		Nickname: req.Nickname,
		Birthday: req.Birthday,
		AboutMe:  req.AboutMe,
	})
	if err != nil {
		return ginx.Result{}, err
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
		ctx.JSON(http.StatusInternalServerError, ginx.Result{
			Code: http.StatusInternalServerError,
			Msg:  "系统错误",
		})
		return
	}
	ctx.JSON(http.StatusOK, profile)
}

// Info 他人主页头部：按 id 取某用户公开信息（昵称/简介/加入时间）。公开可读。
func (h *InternalUserHandler) Info(ctx *gin.Context, req userInfoReq) (ginx.Result, error) {
	u, err := h.userService.Profile(ctx.Request.Context(), req.Id)
	if err != nil {
		h.l.Warn("查询用户公开信息失败", logger.Int64("id", req.Id), logger.Error(err))
		return ginx.NotFound("用户不存在"), nil
	}
	return ginx.Result{Data: userInfoVO{
		Id:        u.Id,
		Nickname:  u.Nickname,
		AboutMe:   u.AboutMe,
		CreatedAt: u.CreatedAt,
	}}, nil
}
