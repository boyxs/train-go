package dao

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var (
	ErrDuplicateUser  = errors.New("此用户已被注册")
	ErrDuplicateEmail = errors.New("邮箱已被注册")
	ErrRecordNotFound = gorm.ErrRecordNotFound
)

type UserDAO interface {
	Insert(ctx context.Context, u User) error
	Update(ctx context.Context, user User) (User, error)
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByPhone(ctx context.Context, phone string) (User, error)
	FindById(ctx context.Context, userid int64) (User, error)
}

type GormUserDAO struct {
	db *gorm.DB
}

func NewGormUserDAO(db *gorm.DB) UserDAO {
	return &GormUserDAO{db: db}
}

func (ud *GormUserDAO) Insert(ctx context.Context, u User) error {
	now := time.Now().UnixMilli() // 毫秒值
	u.CreatedAt = now
	u.UpdatedAt = now
	err := ud.db.WithContext(ctx).Create(&u).Error
	//if mysqlErr, ok := err.(*mysql.MySQLError); ok {}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		const uniqueErrNo uint16 = 1062
		if mysqlErr.Number == uniqueErrNo {
			return ErrDuplicateEmail
		}
	}
	return err
}

func (ud *GormUserDAO) Update(ctx context.Context, user User) (User, error) {
	//now := time.Now().UnixMilli()
	err := ud.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", user.Id).
		Updates(map[string]any{
			"nickname": user.Nickname,
			"birthday": user.Birthday,
			"about_me": user.AboutMe,
			//"updated_at": now,
		}).Error
	if err != nil {
		return User{}, err
	}
	err = ud.db.WithContext(ctx).First(&user, user.Id).Error
	return user, err
}

func (ud *GormUserDAO) FindByEmail(ctx context.Context, email string) (User, error) {
	var u User
	//err := ud.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	err := ud.db.WithContext(ctx).First(&u, "email = ?", email).Error
	return u, err
}

func (ud *GormUserDAO) FindByPhone(ctx context.Context, phone string) (User, error) {
	var u User
	err := ud.db.WithContext(ctx).First(&u, "phone = ?", phone).Error
	return u, err
}

func (ud *GormUserDAO) FindById(ctx context.Context, userid int64) (User, error) {
	var u User
	err := ud.db.WithContext(ctx).First(&u, "id = ?", userid).Error
	return u, err
}

type User struct {
	// gorm.Model 默认包含 ID (uint), CreatedAt, UpdatedAt, DeletedAt
	// 如果用了 gorm.Model，通常就不再手动定义 ID 了
	Id       int64          `gorm:"primaryKey,autoIncrement"`
	Email    sql.NullString `gorm:"unique"`
	Password string         `gorm:"type:varchar(256)"`
	Nickname string         `gorm:"type:varchar(50)"`
	Birthday int64          `gorm:"column:birthday"`
	AboutMe  string         `gorm:"type:text"`
	Phone    sql.NullString `gorm:"unique"`

	// 自动生成时间戳
	CreatedAt int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli"`
}

// TableName 重写表名
func (User) TableName() string {
	return "user"
}
