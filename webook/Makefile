# 模块路径（多模块 go.work：各服务 + pkg/api/shared 独立 module，无 root go.mod）
# goimports -local 用统一前缀即可覆盖全部子模块
MODULE := github.com/boyxs/train-go/webook

# 部署层入口：compose 文件 + .env.local 都在 ../deploy/（项目唯一真相源）
# image 名 webook:local 与 docker-compose.local.yaml 对齐，pull_policy=never
DEPLOY  := ../deploy
COMPOSE := docker compose -p webook-local --env-file $(DEPLOY)/.env.local \
           -f $(DEPLOY)/docker-compose.yaml \
           -f $(DEPLOY)/docker-compose.local.yaml

.PHONY: build dc up down fmt fmt-check wire help

# 宿主机交叉编译 core（internal 模块）Linux 二进制（产物：webook/webook）
# 注：CI/compose 走 internal/Dockerfile 容器内构建，此目标仅本地手动出二进制
build:
	@rm -f webook
	@cd internal && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=k8s -o ../webook .

# 重新构建 webook 镜像 + 重启服务（需先 make build）
# 仅作用 webook 服务，不动 mysql/redis/kafka 等中间件；要起全套用 ../deploy/deploy.sh local
# rmi 强制删旧镜像 — Dockerfile 是 COPY 预编译二进制，layer cache 会让 docker compose build
# 误用旧 binary，强制删后 build 必走完整步骤
dc:
	@docker rmi -f webook:local || true
	@$(COMPOSE) build webook
	@$(COMPOSE) up -d webook

# 一键：编译 + 构建镜像 + 启动 webook 服务
up: build dc

# 停 webook 服务（中间件不动）
down:
	@$(COMPOSE) stop webook

# 格式化所有 Go 文件，本地包分组到第三组
# 没装 goimports 会自动 go install
fmt:
	@command -v goimports >/dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	@goimports -local $(MODULE) -w .
	@echo "✅ goimports 完成"

# CI 检查用：只报告未格式化的文件，不改动
fmt-check:
	@command -v goimports >/dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	@TMPFILE=$$(mktemp) && \
		goimports -local $(MODULE) -l . | tee $$TMPFILE && \
		if [ -s $$TMPFILE ]; then \
			echo "❌ 以上文件 import 未按规范排序，请执行 make fmt"; \
			rm -f $$TMPFILE; \
			exit 1; \
		fi && \
		rm -f $$TMPFILE
	@echo "✅ import 顺序规范"

# 重新生成所有模块的 wire_gen.go（多模块：逐模块 wire ./... 覆盖各自 integration/setup），末尾格式化
wire:
	@for m in internal chat comment interaction migrator relation worker; do \
		echo "→ wire $$m"; (cd $$m && wire ./...) || exit 1; \
	done
	@$(MAKE) fmt

help:
	@echo "make build              - 编译 Linux 二进制（./webook）"
	@echo "make dc                 - 重打 webook 镜像 + docker compose 重启 webook"
	@echo "make up                 - 一键：build + dc（最常用，改完代码运行这条）"
	@echo "make down               - 停 webook 服务（中间件不动）"
	@echo "make fmt                - goimports 格式化（本地包分组）"
	@echo "make fmt-check          - 只检查不改动（CI 用）"
	@echo "make wire               - 重新生成 wire_gen.go + 格式化"
	@echo "make -f mk/k8s.mk help  - K3s 部署相关命令"
	@echo ""
	@echo "前置依赖：../deploy/.env.local 必须存在（首次：cp ../deploy/.env.local.example ../deploy/.env.local）"
	@echo "起全套（含中间件）：cd ../deploy && ./deploy.sh local"
