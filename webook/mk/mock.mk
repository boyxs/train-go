.PHONY: mockgen build
mockgen:
	#Interface-Based Mocking for Upstream Testing
	#handler
	@mockgen -source=./pkg/jwtx/types.go -package=jwtmocks -destination=./pkg/jwtx/mocks/jwt_mock.go
	#service
	@mockgen -source=./internal/service/user.go -package=svcmocks -destination=./internal/service/mocks/user_mock.go
	@mockgen -source=./internal/service/article.go -package=svcmocks -destination=./internal/service/mocks/article_mock.go
	@mockgen -source=./internal/service/code.go -package=svcmocks -destination=./internal/service/mocks/code_mock.go
	@mockgen -source=./internal/service/article_search.go -package=svcmocks -destination=./internal/service/mocks/article_search_mock.go
	@mockgen -source=./internal/service/interaction.go -package=svcmocks -destination=./internal/service/mocks/interaction_mock.go
	#repository
	@mockgen -source=./internal/repository/user.go -package=repomocks -destination=./internal/repository/mocks/user_mock.go
	@mockgen -source=./internal/repository/article_author.go -package=repomocks -destination=./internal/repository/mocks/article_author_mock.go
	@mockgen -source=./internal/repository/article_reader.go -package=repomocks -destination=./internal/repository/mocks/article_reader_mock.go
	@mockgen -source=./internal/repository/code.go -package=repomocks -destination=./internal/repository/mocks/code_mock.go
	@mockgen -source=./internal/repository/article_search.go -package=repomocks -destination=./internal/repository/mocks/article_search_mock.go
	#dao
	@mockgen -source=./internal/repository/dao/user.go -package=daomocks -destination=./internal/repository/dao/mocks/user_mock.go
	@mockgen -source=./internal/repository/dao/article_author.go -package=daomocks -destination=./internal/repository/dao/mocks/article_author_mock.go
	@mockgen -source=./internal/repository/dao/article_reader.go -package=daomocks -destination=./internal/repository/dao/mocks/article_reader_mock.go
	@mockgen -source=./internal/repository/cache/user.go -package=cachemocks -destination=./internal/repository/cache/mocks/user_mock.go
	@mockgen -source=./internal/repository/cache/article.go -package=cachemocks -destination=./internal/repository/cache/mocks/article_mock.go
	@mockgen -source=./internal/repository/cache/code.go -package=cachemocks -destination=./internal/repository/cache/mocks/code_mock.go
	#cache
	@mockgen -package=redismocks -destination=./internal/repository/cache/redismocks/cmdable_mock.go github.com/redis/go-redis/v9 Cmdable
	#sms
	@mockgen -source=./internal/service/sms/types.go -package=smsmocks -destination=./internal/service/sms/mocks/sms_mock.go
	#interaction repository（chat 已拆出独立服务，主仓只剩 interaction repo mock）
	@mockgen -source=./internal/repository/interaction.go -package=repomocks -destination=./internal/repository/mocks/interaction_mock.go
	#click event
	@mockgen -source=./internal/repository/dao/click_event.go -package=daomocks -destination=./internal/repository/dao/mocks/click_event_mock.go
	@mockgen -source=./internal/repository/cache/click_event.go -package=cachemocks -destination=./internal/repository/cache/mocks/click_event_mock.go
	@mockgen -source=./internal/repository/click_event.go -package=repomocks -destination=./internal/repository/mocks/click_event_mock.go
	@mockgen -source=./internal/service/click_event.go -package=svcmocks -destination=./internal/service/mocks/click_event_mock.go
	#article polish
	@mockgen -source=./internal/service/article_polish.go -package=svcmocks -destination=./internal/service/mocks/article_polish_mock.go
	#article ranking
	@mockgen -source=./internal/repository/dao/ranking.go -package=daomocks -destination=./internal/repository/dao/mocks/ranking_mock.go
	@mockgen -source=./internal/repository/cache/ranking.go -package=cachemocks -destination=./internal/repository/cache/mocks/ranking_mock.go
	@mockgen -source=./internal/repository/ranking.go -package=repomocks -destination=./internal/repository/mocks/ranking_mock.go
	@mockgen -source=./internal/service/ranking.go -package=svcmocks -destination=./internal/service/mocks/ranking_mock.go
	#llm（pkg 共享，chat 和 article_polish 都消费）
	@mockgen -source=./pkg/llm/types.go -package=llmmocks -destination=./pkg/llm/mocks/llm_mock.go
	#embedding（search 业务私有依赖，article_search 消费）
	@mockgen -source=./internal/service/embedding/types.go -package=embmocks -destination=./internal/service/embedding/mocks/embedding_mock.go
	#pkg redislockx 分布式锁
	@mockgen -source=./pkg/redislockx/types.go -package=lockmocks -destination=./pkg/redislockx/mocks/lock_mock.go
	#pkg sensitive 敏感词过滤（comment 服务消费）
	@mockgen -source=./pkg/sensitive/types.go -package=sensitivemocks -destination=./pkg/sensitive/mocks/filter.mock.go
	#pkg ratelimit 限流器（comment 服务消费）
	@mockgen -source=./pkg/ratelimit/types.go -package=limitmocks -destination=./pkg/ratelimit/mocks/limiter.mock.go
	#comment 服务（独立 gRPC 微服务：service + repository 接口）
	@mockgen -source=./comment/service/comment.go -package=svcmocks -destination=./comment/service/mocks/comment.mock.go
	@mockgen -source=./comment/repository/comment.go -package=repomocks -destination=./comment/repository/mocks/comment.mock.go
	#comment gRPC client（core HTTP 网关聚合用，reflect 模式 mock 生成的 client 接口）
	@mockgen -destination=./internal/web/grpcmocks/comment_mock.go -package=grpcmocks github.com/webook/api/gen/comment/v1 CommentServiceClient
	#update dependencies
	@go mod tidy
	#格式化生成文件的 import 顺序，避免 CI goimports 校验失败
	@$(MAKE) -f Makefile fmt

build:
	@rm webook || true
	@go mod tidy
	#mac linux windows
	#@GOOS=linux GOARCH=arm go build -tags=dev -o webook .
	#@GOOS=linux GOARCH=amd64 go build -tags=dev -o webook .
	@GOOS=windows GOARCH=amd64 go build -tags=dev -o webook .

# make -f mk/mock.mk mockgen