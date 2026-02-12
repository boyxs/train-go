package web

import (
	"errors"
	"log"
	"net/http"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	//"regexp" 此库不支持 (?=
	regexp "github.com/dlclark/regexp2"
	jwt "github.com/golang-jwt/jwt/v5"
)

const (
	emailExpr = "^\\w+([-+.]\\w+)*@\\w+([-.]\\w+)*\\.\\w+([-.]\\w+)*$"
	// 和上面比起来，用 ` 看起来就比较清爽
	passwordExpr = `^(?=.*[A-Za-z])(?=.*\d)(?=.*[$@$!%*#?&])[A-Za-z\d$@$!%*#?&]{8,}$`
	loginBiz     = "login"
)

type UserHandler struct {
	emailRegexp    *regexp.Regexp
	passwordRegexp *regexp.Regexp
	userService    service.UserService
	codeService    service.CodeService
}

func NewUserHandler(us service.UserService, cs service.CodeService) *UserHandler {
	er := regexp.MustCompile(emailExpr, regexp.None)
	pr := regexp.MustCompile(passwordExpr, regexp.None)
	return &UserHandler{
		emailRegexp:    er,
		passwordRegexp: pr,
		userService:    us,
		codeService:    cs,
	}
}

func (h *UserHandler) RegisterRoutes(server *gin.Engine) {
	ug := server.Group("/user")
	ug.POST("/register", h.Register)
	ug.POST("/login", h.LoginJwt)
	ug.POST("/logout", h.LogoutJwt)
	ug.POST("/edit", h.EditJwt)
	ug.GET("/profile", h.ProfileJwt)
	//ug.POST("/login", h.Login)
	//ug.POST("/logout", h.Logout)
	//ug.POST("/edit", h.Edit)
	//ug.GET("/profile", h.Profile)

	//手机验证码登录相关功能
	ug.POST("/login_sms/code/send", h.SendLoginSMSCode)
	ug.POST("/login_sms", h.LoginSMS)
}

func (h *UserHandler) Register(ctx *gin.Context) {
	type RegisterRequest struct {
		Email           string `json:"email" binding:"required,email"`                      // 必填，且必须是邮箱格式
		Password        string `json:"password" binding:"required,min=8,max=20"`            // 必填，长度8-20位
		ConfirmPassword string `json:"confirmPassword" binding:"required,eqfield=Password"` // 必填，且必须与 Password 字段相等
	}
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	//if errors.Is(err, service.ErrDuplicateEmail) {}
	if err == service.ErrDuplicateEmail {
		ctx.String(http.StatusInternalServerError, "邮箱已被注册")
		return
	}
	if err != nil {
		ctx.String(http.StatusInternalServerError, "系统异常")
		return
	}
	ctx.String(http.StatusOK, "注册成功")
}

func (h *UserHandler) SendLoginSMSCode(ctx *gin.Context) {
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

func (h *UserHandler) LoginSMS(ctx *gin.Context) {
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
		msg := err.Error()
		if msg == "" {
			msg = "系统异常"
		}
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  msg,
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
		msg := err.Error()
		if msg == "" {
			msg = "系统错误"
		}
		ctx.JSON(http.StatusOK, Result{
			Code: 5,
			Msg:  msg,
		})
		return
	}
	err = h.setJwtToken(ctx, user.Id)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "系统异常")
		return
	}
	ctx.JSON(http.StatusOK, Result{
		Msg: "登录成功",
	})
}

func (h *UserHandler) LoginJwt(ctx *gin.Context) {
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
		err := h.setJwtToken(ctx, user.Id)
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

func (h *UserHandler) LogoutJwt(ctx *gin.Context) {
	header := ctx.GetHeader(consts.Authorization)
	if header == "" {
		ctx.JSON(http.StatusOK, gin.H{
			"code": 5,
			"msg":  "退出失败",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "退出成功",
	})
}

func (h *UserHandler) EditJwt(ctx *gin.Context) {
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
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0, "data": user,
	})
}

func (h *UserHandler) ProfileJwt(ctx *gin.Context) {
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

func (h *UserHandler) setJwtToken(ctx *gin.Context, userid int64) error {
	uc := UserClaims{
		Userid:    userid,
		UserAgent: ctx.GetHeader("User-Agent"),
		RegisteredClaims: jwt.RegisteredClaims{
			// 1 分钟过期
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(consts.ExpireTime)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, uc)
	tokenStr, err := token.SignedString(consts.JwtKey)
	if err != nil {
		return err
	}
	ctx.Header(consts.JwtHeader, tokenStr)
	return nil
}

func (h *UserHandler) Login(ctx *gin.Context) {
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

func (h *UserHandler) Logout(ctx *gin.Context) {
	//err := h.ClearToken(ctx)
	session := sessions.Default(ctx)
	session.Delete("userid")
	//session.Clear()
	err := session.Save()
	if err != nil {
		ctx.JSON(http.StatusOK, gin.H{
			"code": 5,
			"msg":  "退出失败",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "退出成功",
	})
}

func (h *UserHandler) Edit(ctx *gin.Context) {
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
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0, "data": user,
	})
}

func (h *UserHandler) Profile(ctx *gin.Context) {
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

type UserClaims struct {
	jwt.RegisteredClaims
	Userid    int64
	UserAgent string
}
