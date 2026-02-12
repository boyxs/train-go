package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/cache"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
)

var (
	ErrDuplicateUser  = dao.ErrDuplicateUser
	ErrDuplicateEmail = dao.ErrDuplicateEmail
	ErrRecordNotFound = dao.ErrRecordNotFound
)

type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	Update(ctx context.Context, user domain.User) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	FindByPhone(ctx context.Context, phone string) (domain.User, error)
	FindById(ctx context.Context, userid int64) (domain.User, error)
}

type RedisUserRepository struct {
	dao   dao.UserDAO
	cache cache.UserCache
}

func NewRedisUserRepository(dao dao.UserDAO, cache cache.UserCache) UserRepository {
	return &RedisUserRepository{
		dao:   dao,
		cache: cache,
	}
}

func (ur *RedisUserRepository) Create(ctx context.Context, user domain.User) error {
	return ur.dao.Insert(ctx, ur.toEntity(user))
}

func (ur *RedisUserRepository) Update(ctx context.Context, user domain.User) (domain.User, error) {
	u, err := ur.dao.Update(ctx, ur.toEntity(user))
	err = ur.cache.Del(ctx, user.Id)
	if err != nil {
		return domain.User{}, err
	}
	return ur.toDomain(u), err
}

func (ur *RedisUserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := ur.dao.FindByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	return ur.toDomain(u), nil
}

func (ur *RedisUserRepository) FindById(ctx context.Context, userid int64) (domain.User, error) {
	cu, err := ur.cache.Get(ctx, userid)
	if err == nil {
		return cu, err
	}
	u, err := ur.dao.FindById(ctx, userid)
	if err != nil {
		return domain.User{}, err
	}
	cu = ur.toDomain(u)
	err = ur.cache.Set(ctx, cu)
	if err != nil {
		fmt.Printf("🚀 ~ file: user.go ~ line 68 ~ err: %#v\n", err)
	}
	return cu, nil
}

func (ur *RedisUserRepository) FindByPhone(ctx context.Context, phone string) (domain.User, error) {
	u, err := ur.dao.FindByPhone(ctx, phone)
	if err != nil {
		return domain.User{}, err
	}
	return ur.toDomain(u), nil
}

func (ur *RedisUserRepository) toDomain(u dao.User) domain.User {
	return domain.User{
		Id:        u.Id,
		Email:     u.Email.String,
		Phone:     u.Phone.String,
		Password:  u.Password,
		Nickname:  u.Nickname,
		Birthday:  time.UnixMilli(u.Birthday).Format(consts.DateOnly),
		AboutMe:   u.AboutMe,
		CreatedAt: time.UnixMilli(u.CreatedAt).Format(consts.DateTimeFull),
		UpdatedAt: time.UnixMilli(u.UpdatedAt).Format(consts.DateTimeFull),
	}
}

func (ur *RedisUserRepository) toEntity(u domain.User) dao.User {
	birthday, _ := time.ParseInLocation(consts.DateOnly, u.Birthday, time.Local)
	return dao.User{
		Id: u.Id,
		Email: sql.NullString{
			String: u.Email,
			Valid:  u.Email != "",
		},
		Phone: sql.NullString{
			String: u.Phone,
			Valid:  u.Phone != "",
		},
		Password: u.Password,
		Birthday: birthday.UnixMilli(),
		AboutMe:  u.AboutMe,
		Nickname: u.Nickname,
	}
}
