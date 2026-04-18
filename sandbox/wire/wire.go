//go:build wireinject

package main

import (
	"wire/repository"
	"wire/repository/dao"

	"github.com/google/wire"
)

func InitUserRepository() *repository.UserRepository {
	wire.Build(InitRedis, dao.NewUserDAO, repository.NewUserRepository)
	return new(repository.UserRepository)
}
