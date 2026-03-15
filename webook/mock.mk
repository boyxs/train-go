.PHONY: mockgen build
mockgen:
	#Interface-Based Mocking for Upstream Testing
	#handler
	@mockgen -source=./internal/web/jwt/types.go -package=jwtmocks -destination=./internal/web/jwt/mocks/jwt_mock.go
	#service
	@mockgen -source=./internal/service/user.go -package=svcmocks -destination=./internal/service/mocks/user_mock.go
	@mockgen -source=./internal/service/code.go -package=svcmocks -destination=./internal/service/mocks/code_mock.go
	#repository
	@mockgen -source=./internal/repository/user.go -package=repomocks -destination=./internal/repository/mocks/user_mock.go
	@mockgen -source=./internal/repository/code.go -package=repomocks -destination=./internal/repository/mocks/code_mock.go
	#dao
	@mockgen -source=./internal/repository/dao/user.go -package=daomocks -destination=./internal/repository/dao/mocks/user_mock.go
	@mockgen -source=./internal/repository/cache/user.go -package=cachemocks -destination=./internal/repository/cache/mocks/user_mock.go
	@mockgen -source=./internal/repository/cache/code.go -package=cachemocks -destination=./internal/repository/cache/mocks/code_mock.go
	#cache
	@mockgen -package=redismocks -destination=./internal/repository/cache/redismocks/cmdable_mock.go github.com/redis/go-redis/v9 Cmdable
	#sms
	@mockgen -source=./internal/service/sms/types.go -package=smsmocks -destination=./internal/service/sms/mocks/sms_mock.go
	#update dependencies
	@go mod tidy

build:
	@rm webook || true
	@go mod tidy
	#mac linux windows
	#@GOOS=linux GOARCH=arm go build -tags=dev -o webook .
	#@GOOS=linux GOARCH=amd64 go build -tags=dev -o webook .
	@GOOS=windows GOARCH=amd64 go build -tags=dev -o webook .

# make mock