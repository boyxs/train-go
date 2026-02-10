package service

import (
	"context"
	"errors"

	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicateEmail        = repository.ErrDuplicateEmail
	ErrRecordNotFound        = repository.ErrRecordNotFound
	ErrInvalidUserOrPassword = errors.New("用户或密码错误")
)

type UserService struct {
	repo repository.IUserRepository
}

func NewUserService(repo repository.IUserRepository) *UserService {
	return &UserService{
		repo: repo,
	}
}

func (us *UserService) Register(ctx context.Context, user domain.User) error {
	// 加密处理
	hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return us.repo.Create(ctx, user)
}

func (us *UserService) Login(ctx context.Context, email string, password string) (domain.User, error) {
	// 查找用户
	user, err := us.repo.FindByEmail(ctx, email)
	if err == ErrRecordNotFound {
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

func (us *UserService) Profile(ctx context.Context, userid int64) (domain.User, error) {
	user, err := us.repo.FindById(ctx, userid)
	if err != nil {
		return domain.User{}, err
	}
	// 不要返回密码
	//user.Password = ""
	return user, nil
}

func (us *UserService) Edit(ctx context.Context, user domain.User) (domain.User, error) {
	user, err := us.repo.Update(ctx, user)
	if err != nil {
		return domain.User{}, err
	}
	// 不要返回密码
	//user.Password = ""
	return user, nil
}
