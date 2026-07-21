package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/boyxs/train-go/webook/pkg/logger"

	"github.com/boyxs/train-go/webook/pkg/redislock"

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
	repo        repository.UserRepository
	redisClient redislock.Client
	l           logger.LoggerX
}

func NewInternalUserService(repo repository.UserRepository, redisClient redislock.Client, l logger.LoggerX) UserService {
	return &InternalUserService{
		repo:        repo,
		redisClient: redisClient,
		l:           l,
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
	lock, ok, err := us.redisClient.TryLock(ctx,
		"user:info:"+strconv.FormatInt(userid, 10),
		redislock.WithWatchdogTimeout(30*time.Second),
		redislock.WithWaitTime(10*time.Second),
	)
	if !ok {
		return domain.User{}, err
	}
	// 拿到锁后再 defer：此时 lock 必非 nil，避免被占/出错时 nil.Unlock 空指针 panic；
	// 用独立 ctx 释放，业务 ctx 已超时/取消也能正常解锁。
	defer func() {
		if err := lock.Unlock(context.Background()); err != nil && !errors.Is(err, redislock.ErrLockNotHeld) {
			us.l.Warn("释放用户信息锁失败", logger.Int64("uid", userid), logger.Error(err))
		}
	}()
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
	us.l.WithContext(ctx).Info("新用户", logger.Field{Key: "wechatAuth", Val: wechatAuth})
	err = us.repo.Create(ctx, domain.User{
		WechatAuth: wechatAuth,
	})
	if err != nil && !errors.Is(err, errs.ErrDuplicateUser) {
		return domain.User{}, err
	}
	return us.repo.FindByWechat(ctx, wechatAuth.OpenId)
}
