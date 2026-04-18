package dao

import (
	"github.com/redis/go-redis/v9"
)

type UserDAO struct {
	cmd redis.Cmdable
}

func NewUserDAO(cmd redis.Cmdable) *UserDAO {
	return &UserDAO{
		cmd: cmd,
	}
}
