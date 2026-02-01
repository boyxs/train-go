package repository

import (
	"context"
	"gitee.com/train-cloud/geektime-basic-go/internal/domain"
	"gitee.com/train-cloud/geektime-basic-go/internal/repository/dao"
	"time"
)

var (
	ErrDuplicateEmail = dao.ErrDuplicateEmail
	ErrRecordNotFound = dao.ErrRecordNotFound
)

type UserRepository struct {
	dao *dao.UserDAO
}

func NewUserRepository(dao *dao.UserDAO) *UserRepository {
	return &UserRepository{
		dao: dao,
	}
}

func (ur *UserRepository) Create(ctx context.Context, user domain.User) error {
	return ur.dao.Insert(ctx, dao.User{
		Email:    user.Email,
		Password: user.Password,
	})
}

func (ur *UserRepository) Update(ctx context.Context, user domain.User) (domain.User, error) {
	layout := "2006-01-02"
	birthday, _ := time.ParseInLocation(layout, user.Birthday, time.Local)
	u, err := ur.dao.Update(ctx, dao.User{
		Id:       user.Id,
		Nickname: user.Nickname,
		Birthday: birthday.UnixMilli(),
		AboutMe:  user.AboutMe,
	})
	return ur.toDomain(u), err
}

func (ur *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := ur.dao.FindByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	return ur.toDomain(u), nil
}

func (ur *UserRepository) FindById(ctx context.Context, userid int64) (domain.User, error) {
	u, err := ur.dao.FindById(ctx, userid)
	if err != nil {
		return domain.User{}, err
	}
	return ur.toDomain(u), nil
}

func (ur *UserRepository) toDomain(u dao.User) domain.User {
	return domain.User{
		Id:       u.Id,
		Email:    u.Email,
		Password: u.Password,
		Nickname: u.Nickname,
		Birthday: time.UnixMilli(u.Birthday).Format("2006-01-02"),
		AboutMe:  u.AboutMe,
		// 2006 年 1 月 2 日 下午 3 点 4 分 5 秒 减 7 小时时区
		// “1 2 3 4 5 6 7” 其中 6 是年份
		// “7” 代表的就是时区（Timezone） 它的不同写法如下
		// 模板占位符  输出示例(以北京时间为例)  含义
		// -0700     +0800                  数字偏移量（无冒号）
		// Z07:00    +08:00                 数字偏移量（带冒号，最标准）
		// MST       CST                    时区缩写名称
		// Z         Z或+08:00              如果是 UTC 则显示 Z，否则显示偏移量
		// 避坑 .000 vs .999
		// .000：如果毫秒是 0，它也会显示（比如 .000）
		// .999：如果毫秒是 0，它会直接省略掉不显示
		CreatedAt: time.UnixMilli(u.CreatedAt).Format("2006-01-02 15:04:05.999 Z07:00"),
		UpdatedAt: time.UnixMilli(u.UpdatedAt).Format("2006-01-02 15:04:05.999 Z07:00"),
	}
}
