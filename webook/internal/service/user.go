package service

import (
	"context"
	"errors"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicateEmail        = repository.ErrDuplicateEmail
	ErrRecordNotFound        = repository.ErrRecordNotFound
	ErrInvalidUserOrPassword = errors.New("用户或密码错误")
)

type UserService interface {
	Register(ctx context.Context, user domain.User) error
	Login(ctx context.Context, email string, password string) (domain.User, error)
	Profile(ctx context.Context, userid int64) (domain.User, error)
	Edit(ctx context.Context, user domain.User) (domain.User, error)
	FindOrCreate(ctx context.Context, phone string) (domain.User, error)
	SetJwtToken(ctx *gin.Context, userid int64) error
}
type InternalUserService struct {
	repo repository.UserRepository
}

func (us *InternalUserService) Register(ctx context.Context, user domain.User) error {
	// 加密处理
	hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return us.repo.Create(ctx, user)
}

func (us *InternalUserService) Login(ctx context.Context, email string, password string) (domain.User, error) {
	// 查找用户
	user, err := us.repo.FindByEmail(ctx, email)
	if errors.Is(err, ErrRecordNotFound) {
		return domain.User{}, ErrInvalidUserOrPassword
	}
	if err != nil {
		return domain.User{}, err
	}
	// 解密处理
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return domain.User{}, ErrInvalidUserOrPassword
	}
	return user, err
}

func (us *InternalUserService) Profile(ctx context.Context, userid int64) (domain.User, error) {
	user, err := us.repo.FindById(ctx, userid)
	if err != nil {
		return domain.User{}, err
	}
	// 不要返回密码
	//user.Password = ""
	return user, nil
}

func (us *InternalUserService) Edit(ctx context.Context, user domain.User) (domain.User, error) {
	user, err := us.repo.Update(ctx, user)
	if err != nil {
		return domain.User{}, err
	}
	// 不要返回密码
	//user.Password = ""
	return user, nil
}

func (us *InternalUserService) FindOrCreate(ctx context.Context, phone string) (domain.User, error) {
	u, err := us.repo.FindByPhone(ctx, phone)
	if !errors.Is(err, repository.ErrRecordNotFound) {
		// 有两种情况
		// err == nil, u 是可用的
		// err != nil，系统错误，
		return u, err
	}
	err = us.repo.Create(ctx, domain.User{Phone: phone})
	// 有两种可能，一种是 err 恰好是唯一索引冲突（phone）
	// 一种是 err != nil，系统错误
	if err != nil && !errors.Is(err, repository.ErrDuplicateUser) {
		return domain.User{}, err
	}
	// 要么 err ==nil，要么ErrDuplicateUser，也代表用户存在
	// 主从延迟，理论上来讲，强制走主库
	return us.repo.FindByPhone(ctx, phone)
}

func (us *InternalUserService) SetJwtToken(ctx *gin.Context, userid int64) error {
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

func NewInternalUserService(repo repository.UserRepository) UserService {
	return &InternalUserService{
		repo: repo,
	}
}

type UserClaims struct {
	jwt.RegisteredClaims
	Userid    int64
	UserAgent string
}
