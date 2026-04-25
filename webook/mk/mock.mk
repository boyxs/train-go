.PHONY: mockgen build
mockgen:
	#Interface-Based Mocking for Upstream Testing
	#handler
	@mockgen -source=./internal/web/jwt/types.go -package=jwtmocks -destination=./internal/web/jwt/mocks/jwt_mock.go
	#service
	@mockgen -source=./internal/service/user.go -package=svcmocks -destination=./internal/service/mocks/user_mock.go
	@mockgen -source=./internal/service/article.go -package=svcmocks -destination=./internal/service/mocks/article_mock.go
	@mockgen -source=./internal/service/code.go -package=svcmocks -destination=./internal/service/mocks/code_mock.go
	@mockgen -source=./internal/service/article_search.go -package=svcmocks -destination=./internal/service/mocks/article_search_mock.go
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
	#chat repository
	@mockgen -source=./internal/repository/chat_conversation.go -package=repomocks -destination=./internal/repository/mocks/chat_conversation_mock.go
	@mockgen -source=./internal/repository/chat_message.go -package=repomocks -destination=./internal/repository/mocks/chat_message_mock.go
	@mockgen -source=./internal/repository/interaction.go -package=repomocks -destination=./internal/repository/mocks/interaction_mock.go
	@mockgen -source=./internal/service/chat_tools.go -package=svcmocks -destination=./internal/service/mocks/chat_tools_mock.go
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
	#ai
	@mockgen -source=./internal/service/ai/llm.go -package=aimocks -destination=./internal/service/ai/mocks/llm_mock.go
	#embedding
	@mockgen -source=./internal/service/ai/embedding/types.go -package=embmocks -destination=./internal/service/ai/embedding/mocks/embedding_mock.go
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