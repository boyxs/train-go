package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	svcmocks "gitee.com/train-cloud/geektime-basic-go/internal/service/mocks"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	jwtmocks "gitee.com/train-cloud/geektime-basic-go/internal/web/jwt/mocks"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"
)

func TestPasswordEncrypt(t *testing.T) {
	password := []byte("@12345678a")
	encrypted, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	assert.NoError(t, err)
	fmt.Printf("🚀 ~ file: user_test.go ~ line 13 ~ encrypted: %#v\n", string(encrypted))
	err = bcrypt.CompareHashAndPassword(encrypted, password)
	assert.NoError(t, err)
}

func TestEmail(t *testing.T) {
	testCases := []struct {
		name  string
		email string
		match bool
	}{
		{
			name:  "不带@",
			email: "123456",
			match: false,
		},
		{
			name:  "带@ 但是没后缀",
			email: "123456@",
			match: false,
		},
		{
			name:  "合法邮箱",
			email: "123456@qq.com",
			match: true,
		},
	}

	h := NewInternalUserHandler(nil, nil, nil, logger.NewNopLogger())
	for _, ts := range testCases {
		t.Run(ts.name, func(t *testing.T) {
			matchStr, err := h.(*InternalUserHandler).emailRegexp.MatchString(ts.email)
			require.NoError(t, err)
			assert.Equal(t, ts.match, matchStr)
		})

	}
}

func TestMock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	userService := svcmocks.NewMockUserService(ctrl)
	userService.EXPECT().Register(gomock.Any(), domain.User{
		Id:    1,
		Email: "123456@qq.com",
	}).Return(errors.New("mock error"))
	err := userService.Register(context.Background(), domain.User{
		Id:    1,
		Email: "123456@qq.com",
	})
	t.Error(err)
}

func TestInternalUserHandler_Register(t *testing.T) {
	testCases := []struct {
		name       string
		mock       func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService)
		reqBuilder func(t *testing.T) *http.Request
		wantCode   int
		wantBody   string
	}{
		{
			name: "注册成功",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				userService.EXPECT().Register(gomock.Any(), domain.User{
					Email:    "123456@qq.com",
					Password: "@12345678a",
				}).Return(nil)
				return nil, userService, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq.com",
"password": "@12345678a",
"confirmPassword": "@12345678a"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
			wantBody: "注册成功",
		},
		{
			name: "ShouldBindJSON 出错",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				return nil, nil, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq.com",
"pass
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "非法邮箱格式",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				return nil, nil, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq",
"password": "@12345678a",
"confirmPassword": "@12345678a"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
			wantBody: "非法邮箱格式",
		},
		{
			name: "两次输入密码不匹配",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				return nil, nil, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq.com",
"password": "@12345678a",
"confirmPassword": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
			wantBody: "两次输入密码不匹配",
		},
		{
			name: "密码格式不对",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				return nil, nil, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq.com",
"password": "123456",
"confirmPassword": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
			wantBody: "密码必须包含字母、数字、特殊字符，并且不少于八位",
		},
		{
			name: "邮箱已被注册",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				userService.EXPECT().Register(gomock.Any(), domain.User{
					Email:    "123456@qq.com",
					Password: "@12345678a",
				}).Return(service.ErrDuplicateEmail)
				return nil, userService, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq.com",
"password": "@12345678a",
"confirmPassword": "@12345678a"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
			wantBody: "邮箱已被注册",
		},
		{
			name: "系统异常",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				userService.EXPECT().Register(gomock.Any(), domain.User{
					Email:    "123456@qq.com",
					Password: "@12345678a",
				}).Return(errors.New("系统异常"))
				return nil, userService, nil
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(
					http.MethodPost, "/user/register", bytes.NewReader([]byte(`{
"email": "123456@qq.com",
"password": "@12345678a",
"confirmPassword": "@12345678a"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
			wantBody: "系统异常",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			//构建Handler
			jwtHandler, userService, codeService := tc.mock(ctrl)
			h := NewInternalUserHandler(jwtHandler, userService, codeService, logger.NewNopLogger())
			//启动服务，注册路由
			server := gin.Default()
			h.RegisterRoutes(server)
			//准备Req和记录者
			req := tc.reqBuilder(t)
			recorder := httptest.NewRecorder()
			//执行请求
			server.ServeHTTP(recorder, req)
			//断言结果
			assert.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantBody, recorder.Body.String())
		})
	}
}

func TestInternalUserHandler_LoginSMS(t *testing.T) {
	testCases := []struct {
		name string

		mock       func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService)
		reqBuilder func(t *testing.T) *http.Request

		ctx *gin.Context

		wantCode   int
		wantBody   string
		wantResult Result
	}{
		{
			name: "登录成功",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				jwtHandler := jwtmocks.NewMockJwtHandler(ctrl)
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				codeService.EXPECT().Verify(gomock.Any(), loginBiz, "18608261234", "123456").
					Return(true, nil)
				userService.EXPECT().FindOrCreate(gomock.Any(), "18608261234").
					Return(domain.User{
						Id:       102,
						Nickname: "tommy",
						AboutMe:  "say my name",
						Phone:    "18608261234",
					}, nil)
				jwtHandler.EXPECT().SetLoginToken(gomock.Any(), int64(102)).
					Return(nil)
				return jwtHandler, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Msg: "登录成功"},
		},
		{
			name: "ShouldBindJSON 错误",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				return nil, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "1
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusOK,
		},
		{
			name: "验证码验证太频繁",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				codeService.EXPECT().Verify(gomock.Any(), loginBiz, "18608261234", "123456").
					Return(false, service.ErrCodeVerifyTooMany)
				return nil, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Code: 5, Msg: service.ErrCodeVerifyTooMany.Error()},
		},
		{
			name: "验证码错误，请重新输入",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				codeService.EXPECT().Verify(gomock.Any(), loginBiz, "18608261234", "123456").
					Return(false, nil)
				return nil, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Code: 4, Msg: "验证码错误，请重新输入"},
		},
		{
			name: "用户未找到",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				codeService.EXPECT().Verify(gomock.Any(), loginBiz, "18608261234", "123456").
					Return(true, nil)
				userService.EXPECT().FindOrCreate(gomock.Any(), "18608261234").
					Return(domain.User{}, service.ErrRecordNotFound)
				return nil, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Code: 5, Msg: service.ErrRecordNotFound.Error()},
		},
		{
			name: "用户已存在",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				codeService.EXPECT().Verify(gomock.Any(), loginBiz, "18608261234", "123456").
					Return(true, nil)
				userService.EXPECT().FindOrCreate(gomock.Any(), "18608261234").
					Return(domain.User{}, repository.ErrDuplicateUser)
				return nil, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode:   http.StatusOK,
			wantResult: Result{Code: 5, Msg: repository.ErrDuplicateUser.Error()},
		},
		{
			name: "系统异常",
			mock: func(ctrl *gomock.Controller) (jwt.JwtHandler, service.UserService, service.CodeService) {
				jwtHandler := jwtmocks.NewMockJwtHandler(ctrl)
				userService := svcmocks.NewMockUserService(ctrl)
				codeService := svcmocks.NewMockCodeService(ctrl)
				codeService.EXPECT().Verify(gomock.Any(), loginBiz, "18608261234", "123456").
					Return(true, nil)
				userService.EXPECT().FindOrCreate(gomock.Any(), "18608261234").
					Return(domain.User{
						Id:       102,
						Nickname: "tommy",
						AboutMe:  "say my name",
						Phone:    "18608261234",
					}, nil)
				jwtHandler.EXPECT().SetLoginToken(gomock.Any(), int64(102)).
					Return(errors.New("token expired"))
				return jwtHandler, userService, codeService
			},
			reqBuilder: func(t *testing.T) *http.Request {
				req, err := http.NewRequest(http.MethodPost, "/user/login_sms", bytes.NewReader([]byte(`{
"phone": "18608261234",
"code": "123456"
}`)))
				req.Header.Add("Content-Type", "application/json")
				assert.NoError(t, err)
				return req
			},
			wantCode: http.StatusInternalServerError,
			wantBody: "系统异常",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			jwtHandler, userService, codeService := tc.mock(ctrl)
			h := NewInternalUserHandler(jwtHandler, userService, codeService, logger.NewNopLogger())
			server := gin.Default()
			h.RegisterRoutes(server)
			req := tc.reqBuilder(t)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, req)
			assert.Equal(t, tc.wantCode, recorder.Code)
			contentType := recorder.Header().Get("Content-Type")
			if strings.Contains(contentType, "application/json") {
				//反序列化
				var result Result
				err := json.Unmarshal(recorder.Body.Bytes(), &result)
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, result)
			} else {
				assert.Equal(t, tc.wantBody, recorder.Body.String())
			}
		})
	}
}
