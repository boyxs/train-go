package web

import (
	"errors"
	"log"
	"net/http"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
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
	Register(ctx *gin.Context)
	SendLoginSMSCode(ctx *gin.Context)
	LoginSMS(ctx *gin.Context)
	Login(ctx *gin.Context)
	Logout(ctx *gin.Context)
	Edit(ctx *gin.Context)
	Profile(ctx *gin.Context)
	//Session Operations
	LoginSS(ctx *gin.Context)
	LogoutSS(ctx *gin.Context)
	EditSS(ctx *gin.Context)
	ProfileSS(ctx *gin.Context)
}

type InternalUserHandler struct {
	jwt.JwtHandler
	emailRegexp    *regexp.Regexp
	passwordRegexp *regexp.Regexp
	userService    service.UserService
	codeService    service.CodeService
}

type UserClaims = jwt.UserClaims

func NewInternalUserHandler(hdl jwt.JwtHandler, us service.UserService, cs service.CodeService) UserHandler {
	er := regexp.MustCompile(emailExpr, regexp.None)
	pr := regexp.MustCompile(passwordExpr, regexp.None)
	return &InternalUserHandler{
		JwtHandler:     hdl,
		emailRegexp:    er,
		passwordRegexp: pr,
		userService:    us,
		codeService:    cs,
	}
}

func (h *InternalUserHandler) RegisterRoutes(server *gin.Engine) {
	ug := server.Group("/user")
	ug.POST("/register", h.Register)
	ug.POST("/login", h.Login)
	ug.POST("/logout", h.Logout)
	ug.POST("/edit", h.Edit)
	ug.GET("/refresh_token", h.RefreshToken)
	ug.GET("/profile", h.Profile)
	//ug.POST("/login", h.LoginSS)
	//ug.POST("/logout", h.LogoutSS)
	//ug.POST("/edit", h.EditSS)
	//ug.GET("/profile", h.ProfileSS)

	//手机验证码登录相关功能
	ug.POST("/login_sms/code/send", h.SendLoginSMSCode)
	ug.POST("/login_sms", h.LoginSMS)
}

func (h *InternalUserHandler) Register(ctx *gin.Context) {
	type RegisterRequest struct {
		//Email           string `json:"email" binding:"required,email"`                      // 必填，且必须是邮箱格式
		//Password        string `json:"password" binding:"required,min=8,max=20"`            // 必填，长度8-20位
		//ConfirmPassword string `json:"confirmPassword" binding:"required,eqfield=Password"` // 必填，且必须与 Password 字段相等
		Email           string `json:"email" binding:"required"`           // 必填，这里使用后面逻辑校验
		Password        string `json:"password" binding:"required"`        // 必填，这里使用后面逻辑校验
		ConfirmPassword string `json:"confirmPassword" binding:"required"` // 必填，这里使用后面逻辑校验
	}
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	// &syntax.Error{Code:"invalid or unsupported Perl syntax", Expr:"(?="}
	// match, err := regexp.Match(passwordRegexPattern, []byte("000"))

	isEmail, err := h.emailRegexp.MatchString(req.Email)
	if err != nil {
		ctx.String(http.StatusOK, "系统错误")
		return
	}
	if !isEmail {
		ctx.String(http.StatusOK, "非法邮箱格式")
		return
	}
	if req.Password != req.ConfirmPassword {
		ctx.String(http.StatusOK, "两次输入密码不匹配")
		return
	}
	isPassword, err := h.passwordRegexp.MatchString(req.Password)
	if err != nil {
		ctx.String(http.StatusOK, "系统错误")
		return
	}
	if !isPassword {
		ctx.String(http.StatusOK, "密码必须包含字母、数字、特殊字符，并且不少于八位")
		return
	}
	//ctx.String(http.StatusOK, "注册成功")
	err = h.userService.Register(ctx, domain.User{
		Email:    req.Email,
		Password: req.Password,
	})
	if errors.Is(err, service.ErrDuplicateEmail) {
		ctx.String(http.StatusOK, "邮箱已被注册")
		return
	}
	if err != nil {
		ctx.String(http.StatusOK, "系统异常")
		return
	}
	ctx.String(http.StatusOK, "注册成功")
}

func (h *InternalUserHandler) SendLoginSMSCode(ctx *gin.Context) {
	type CodeRequest struct {
		Phone string `json:"phone"`
	}
	var req CodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		return
	}
	if req.Phone == "" {
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "请输入手机号码",
		})
		return
	}
	err := h.codeService.Send(ctx, loginBiz, req.Phone)
	switch {
	case err == nil:
		ctx.JSON(http.StatusOK, Result{
			Msg: "发送成功",
		})
	case errors.Is(err, service.ErrCodeSendTooMany):
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "短信发送太频繁，请稍后再试",
		})
	default:
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  "系统错误",
		})
		//记录日志
		log.Println(err)
	}
}

func (h *InternalUserHandler) LoginSMS(ctx *gin.Context) {
	type LoginRequest struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		return
	}
	ok, err := h.codeService.Verify(ctx, loginBiz, req.Phone, req.Code)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  err.Error(),
		})
		return
	}
	if !ok {
		ctx.JSON(http.StatusOK, Result{
			Code: 4,
			Msg:  "验证码错误，请重新输入",
		})
		return
	}
	user, err := h.userService.FindOrCreate(ctx, req.Phone)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  err.Error(),
		})
		return
	}
	err = h.SetLoginToken(ctx, user.Id)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "系统异常")
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "登录成功",
	})
}

func (h *InternalUserHandler) Login(ctx *gin.Context) {
	type LoginRequest struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		return
	}
	user, err := h.userService.Login(ctx, req.Email, req.Password)
	switch err {
	case nil:
		err := h.SetLoginToken(ctx, user.Id)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "系统异常")
			return
		}
		ctx.String(http.StatusOK, "登录成功")
	case service.ErrInvalidUserOrPassword:
		ctx.String(http.StatusOK, "用户名或密码错误")
	default:
		ctx.String(http.StatusOK, "系统错误")
	}
}

func (h *InternalUserHandler) Logout(ctx *gin.Context) {
	err := h.ClearToken(ctx)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "退出失败"})
		return
	}
	ctx.JSON(http.StatusOK, Result{Code: 0, Msg: "退出成功"})
}

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
	err = h.CheckSession(ctx, rc.Ssid)
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	err = h.SetAccessToken(ctx, rc.Userid, rc.Ssid)
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "OK",
	})
}

func (h *InternalUserHandler) Edit(ctx *gin.Context) {
	type EditRequest struct {
		Nickname string `json:"nickname"`
		Birthday string `json:"birthday"`
		AboutMe  string `json:"aboutMe"`
	}
	var req EditRequest
	err := ctx.Bind(&req)
	if err != nil {
		return
	}
	uc, ok := ctx.MustGet(consts.UserKey).(UserClaims)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, "请先登录")
		return
	}
	user, err := h.userService.Edit(ctx, domain.User{
		Id:       uc.Userid,
		Nickname: req.Nickname,
		Birthday: req.Birthday,
		AboutMe:  req.AboutMe,
	})
	if err != nil {
		return
	}
	ctx.JSON(http.StatusOK, Result{Code: 0, Data: user})
}

func (h *InternalUserHandler) Profile(ctx *gin.Context) {
	uc, ok := ctx.MustGet(consts.UserKey).(UserClaims)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, "请先登录")
		return
	}
	profile, err := h.userService.Profile(ctx, uc.Userid)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "系统异常")
		return
	}
	ctx.JSON(http.StatusOK, profile)
}

// ==================Session Operations==================

func (h *InternalUserHandler) LoginSS(ctx *gin.Context) {
	type LoginRequest struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		return
	}
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
		err := session.Save()
		if err != nil {
			ctx.String(http.StatusInternalServerError, "系统异常")
			return
		}
		ctx.String(http.StatusOK, "登录成功")
	case service.ErrInvalidUserOrPassword:
		ctx.String(http.StatusOK, "用户名或密码错误")
	default:
		ctx.String(http.StatusOK, "系统错误")
	}
}

func (h *InternalUserHandler) LogoutSS(ctx *gin.Context) {
	//err := h.ClearToken(ctx)
	session := sessions.Default(ctx)
	session.Delete("userid")
	//session.Clear()
	err := session.Save()
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "退出失败"})
		return
	}
	ctx.JSON(http.StatusOK, Result{Code: 0, Msg: "退出成功"})
}

func (h *InternalUserHandler) EditSS(ctx *gin.Context) {
	type EditRequest struct {
		Nickname string `json:"nickname"`
		Birthday string `json:"birthday"`
		AboutMe  string `json:"aboutMe"`
	}
	var req EditRequest
	err := ctx.Bind(&req)
	if err != nil {
		return
	}
	session := sessions.Default(ctx)
	userid := session.Get("userid")
	user, err := h.userService.Edit(ctx, domain.User{
		Id:       userid.(int64),
		Nickname: req.Nickname,
		Birthday: req.Birthday,
		AboutMe:  req.AboutMe,
	})
	if err != nil {
		return
	}
	ctx.JSON(http.StatusOK, Result{Code: 0, Data: user})
}

func (h *InternalUserHandler) ProfileSS(ctx *gin.Context) {
	session := sessions.Default(ctx)
	val := session.Get("userid")
	userid, ok := val.(int64)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, "请先登录")
		return
	}
	profile, err := h.userService.Profile(ctx, userid)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "系统异常")
		return
	}
	ctx.JSON(http.StatusOK, profile)
}
