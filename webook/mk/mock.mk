.PHONY: mockgen build
# 多模块：mockgen 必须在目标接口所属 module 目录内执行（go.work 提供跨模块解析）；
# 每个 recipe 行是独立 shell，故每行各自 cd 进对应 module，-source/-destination 相对该 module 根。
mockgen:
	#── pkg（跨服务共享工具）
	@cd pkg && mockgen -source=./jwtx/types.go -package=jwtmocks -destination=./jwtx/mocks/jwt_mock.go
	@cd pkg && mockgen -source=./llm/types.go -package=llmmocks -destination=./llm/mocks/llm_mock.go
	@cd pkg && mockgen -source=./redislock/redislock.go -package=lockmocks -destination=./redislock/mocks/lock_mock.go
	@cd pkg && mockgen -source=./sensitive/types.go -package=sensitivemocks -destination=./sensitive/mocks/filter.mock.go
	@cd pkg && mockgen -source=./ratelimit/types.go -package=limitmocks -destination=./ratelimit/mocks/limiter.mock.go
	#── internal（core：service）
	@cd internal && mockgen -source=./service/user.go -package=svcmocks -destination=./service/mocks/user_mock.go
	@cd internal && mockgen -source=./service/article.go -package=svcmocks -destination=./service/mocks/article_mock.go
	@cd internal && mockgen -source=./service/code.go -package=svcmocks -destination=./service/mocks/code_mock.go
	@cd internal && mockgen -source=./service/article_search.go -package=svcmocks -destination=./service/mocks/article_search_mock.go
	@cd internal && mockgen -source=./service/interaction.go -package=svcmocks -destination=./service/mocks/interaction_mock.go
	@cd internal && mockgen -source=./service/comment.go -package=svcmocks -destination=./service/mocks/comment_mock.go
	@cd internal && mockgen -source=./service/relation.go -package=svcmocks -destination=./service/mocks/relation_mock.go
	@cd internal && mockgen -source=./service/click_event.go -package=svcmocks -destination=./service/mocks/click_event_mock.go
	@cd internal && mockgen -source=./service/article_polish.go -package=svcmocks -destination=./service/mocks/article_polish_mock.go
	@cd internal && mockgen -source=./service/ranking.go -package=svcmocks -destination=./service/mocks/ranking_mock.go
	@cd internal && mockgen -source=./service/sms/types.go -package=smsmocks -destination=./service/sms/mocks/sms_mock.go
	@cd internal && mockgen -source=./service/embedding/types.go -package=embmocks -destination=./service/embedding/mocks/embedding_mock.go
	#── internal（core：repository）
	@cd internal && mockgen -source=./repository/user.go -package=repomocks -destination=./repository/mocks/user_mock.go
	@cd internal && mockgen -source=./repository/article_author.go -package=repomocks -destination=./repository/mocks/article_author_mock.go
	@cd internal && mockgen -source=./repository/article_reader.go -package=repomocks -destination=./repository/mocks/article_reader_mock.go
	@cd internal && mockgen -source=./repository/code.go -package=repomocks -destination=./repository/mocks/code_mock.go
	@cd internal && mockgen -source=./repository/article_search.go -package=repomocks -destination=./repository/mocks/article_search_mock.go
	@cd internal && mockgen -source=./repository/click_event.go -package=repomocks -destination=./repository/mocks/click_event_mock.go
	@cd internal && mockgen -source=./repository/ranking.go -package=repomocks -destination=./repository/mocks/ranking_mock.go
	#── internal（core：dao）
	@cd internal && mockgen -source=./repository/dao/user.go -package=daomocks -destination=./repository/dao/mocks/user_mock.go
	@cd internal && mockgen -source=./repository/dao/article_author.go -package=daomocks -destination=./repository/dao/mocks/article_author_mock.go
	@cd internal && mockgen -source=./repository/dao/article_reader.go -package=daomocks -destination=./repository/dao/mocks/article_reader_mock.go
	@cd internal && mockgen -source=./repository/dao/click_event.go -package=daomocks -destination=./repository/dao/mocks/click_event_mock.go
	@cd internal && mockgen -source=./repository/dao/ranking.go -package=daomocks -destination=./repository/dao/mocks/ranking_mock.go
	#── internal（core：cache）
	@cd internal && mockgen -source=./repository/cache/user.go -package=cachemocks -destination=./repository/cache/mocks/user_mock.go
	@cd internal && mockgen -source=./repository/cache/article.go -package=cachemocks -destination=./repository/cache/mocks/article_mock.go
	@cd internal && mockgen -source=./repository/cache/code.go -package=cachemocks -destination=./repository/cache/mocks/code_mock.go
	@cd internal && mockgen -source=./repository/cache/click_event.go -package=cachemocks -destination=./repository/cache/mocks/click_event_mock.go
	@cd internal && mockgen -source=./repository/cache/ranking.go -package=cachemocks -destination=./repository/cache/mocks/ranking_mock.go
	#── internal reflect-mode（外部 go-redis / 下游 gRPC client 接口）
	@cd internal && mockgen -package=redismocks -destination=./repository/cache/redismocks/cmdable_mock.go github.com/redis/go-redis/v9 Cmdable
	@cd internal && mockgen -destination=./web/grpcmocks/comment_mock.go -package=grpcmocks github.com/boyxs/train-go/webook/api/gen/comment/v1 CommentServiceClient
	#── comment（独立 gRPC 微服务：service + repository 接口）
	@cd comment && mockgen -source=./service/comment.go -package=svcmocks -destination=./service/mocks/comment.mock.go
	@cd comment && mockgen -source=./repository/comment.go -package=repomocks -destination=./repository/mocks/comment.mock.go
	#── 各 module tidy（mock 依赖已在图中，通常无新增；tidy 触及的 3 个 module 保险）
	@cd internal && go mod tidy
	@cd pkg && go mod tidy
	@cd comment && go mod tidy
	#── 格式化生成文件 import 顺序，避免 CI goimports 校验失败
	@$(MAKE) -f Makefile fmt

# 本地手动出 core（internal 模块）二进制（Windows，调试用）
build:
	@rm -f webook.exe
	@cd internal && GOOS=windows GOARCH=amd64 go build -tags=dev -o ../webook.exe .

# make -f mk/mock.mk mockgen
