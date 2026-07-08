package service

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/errs"
	"github.com/boyxs/train-go/webook/internal/repository"
)

type UserService interface {
	Register(ctx context.Context, user domain.User) error
	Login(ctx context.Context, email string, password string) (domain.User, error)
	Profile(ctx context.Context, userid int64) (domain.User, error)
	// FindByIds 批量取用户，返回 map[uid]User（评论区聚合 uid→昵称用）
	FindByIds(ctx context.Context, ids []int64) (map[int64]domain.User, error)
	Edit(ctx context.Context, user domain.User) (domain.User, error)
	FindOrCreate(ctx context.Context, phone string) (domain.User, error)
	FindOrCreateByWechat(ctx context.Context, wechatAuth domain.WechatAuth) (domain.User, error)
}
type InternalUserService struct {
	repo repository.UserRepository
}

func NewInternalUserService(repo repository.UserRepository) UserService {
	return &InternalUserService{
		repo: repo,
	}
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
	if errors.Is(err, errs.ErrRecordNotFound) {
		return domain.User{}, errs.ErrInvalidUserOrPassword
	}
	if err != nil {
		return domain.User{}, err
	}
	// 解密处理
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return domain.User{}, errs.ErrInvalidUserOrPassword
	}
	return user, err
}

func (us *InternalUserService) Profile(ctx context.Context, userid int64) (domain.User, error) {
	user, err := us.repo.FindById(ctx, userid)
	if err != nil {
		return domain.User{}, err
	}
	// 不要返回密码
	user.Password = ""
	return user, nil
}

func (us *InternalUserService) FindByIds(ctx context.Context, ids []int64) (map[int64]domain.User, error) {
	if len(ids) == 0 {
		return map[int64]domain.User{}, nil
	}
	users, err := us.repo.FindByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]domain.User, len(users))
	for _, u := range users {
		result[u.Id] = u
	}
	return result, nil
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
	if !errors.Is(err, errs.ErrRecordNotFound) {
		// 有两种情况
		// err == nil, u 是可用的
		// err != nil，系统错误，
		return u, err
	}
	err = us.repo.Create(ctx, domain.User{Phone: phone})
	// 有两种可能，一种是 err 恰好是唯一索引冲突（phone）
	// 一种是 err != nil，系统错误
	if err != nil && !errors.Is(err, errs.ErrDuplicateUser) {
		return domain.User{}, err
	}
	// 要么 err ==nil，要么ErrDuplicateUser，也代表用户存在
	// 主从延迟，理论上来讲，强制走主库
	return us.repo.FindByPhone(ctx, phone)
}

func (us *InternalUserService) FindOrCreateByWechat(ctx context.Context, wechatAuth domain.WechatAuth) (domain.User, error) {
	u, err := us.repo.FindByWechat(ctx, wechatAuth.OpenId)
	if !errors.Is(err, errs.ErrRecordNotFound) {
		return u, err
	}
	// 创建一个新用户
	// JSON 格式的 wechatAuth
	zap.L().Info("新用户", zap.Any("wechatAuth", wechatAuth))
	//us.logger.Info("新用户", zap.Any("wechatAuth", wechatAuth))
	err = us.repo.Create(ctx, domain.User{
		WechatAuth: wechatAuth,
	})
	if err != nil && !errors.Is(err, errs.ErrDuplicateUser) {
		return domain.User{}, err
	}
	return us.repo.FindByWechat(ctx, wechatAuth.OpenId)
}
