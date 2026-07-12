# 模块路径（多模块 go.work：各服务 + pkg/api/shared 独立 module，无 root go.mod）
# goimports -local 用统一前缀即可覆盖全部子模块
MODULE := github.com/boyxs/train-go/webook

# 全部 12 个 module（叶子 + 服务）——verify 等逐模块操作用
MODULES := api shared pkg internal chat comment interaction migrator relation worker tag search

# 部署层入口：compose 文件 + .env.local 都在 ../deploy/（项目唯一真相源）
# image 名 webook:local 与 docker-compose.local.yaml 对齐，pull_policy=never
DEPLOY  := ../deploy
COMPOSE := docker compose -p webook-local --env-file $(DEPLOY)/.env.local \
           -f $(DEPLOY)/docker-compose.yaml \
           -f $(DEPLOY)/docker-compose.local.yaml

.PHONY: build dc up down fmt fmt-check verify wire backfill-search help

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

# 提交前全量校验（多模块）：格式 + 各模块 build/vet（workspace）+ GOWORK=off 构建
# 为何两种模式都跑：workspace 用 go.work 的 use 会掩盖单模块 go.mod 里写错的 replace
# （如 ../api 误写成 ../api0），只有 GOWORK=off（= Docker/CI 的解析方式）才暴露。
# 注：recipe 内 echo 一律用 ASCII——Windows GBK 终端下中文会 mojibake 且可能破坏 shell 引号。
verify: fmt-check
	@echo "-- [1/2] build+vet each module (workspace) --"
	@for m in $(MODULES); do \
		printf '  %-12s ' "$$m"; \
		(cd $$m && go build ./... >/dev/null && go vet ./... >/dev/null) && echo ok \
			|| { echo "FAIL: $$m build/vet"; exit 1; }; \
	done
	@echo "-- [2/2] GOWORK=off build each module (Docker/CI parity; catches bad replace) --"
	@for m in $(MODULES); do \
		printf '  %-12s ' "$$m"; \
		(cd $$m && GOWORK=off GOFLAGS=-mod=readonly go build ./... >/dev/null) && echo ok \
			|| { echo "FAIL: $$m GOWORK=off build -- check go.mod replace path / dep drift"; exit 1; }; \
	done
	@echo "OK: fmt + 12 modules build under workspace and GOWORK=off"

# 重新生成所有模块的 wire_gen.go（逐模块 `wire ./...`，覆盖模块根 + integration/setup 的全部 injector）。
# 每个 test-only 的 integration 包都放了 integration.go 锚文件（见 CLAUDE.md「集成测试规范」），故 `wire ./...`
# 不会再被空 GoFiles 目录绊倒（否则会在 injector 检测前的 detectOutputDir 阶段报
# "no files to derive output directory from"、令整条命令退 1），现全成功退 0。末尾格式化。
wire:
	@for m in internal chat comment interaction migrator relation worker tag search; do \
		echo "→ wire $$m"; (cd $$m && wire ./...) || exit 1; \
	done
	@$(MAKE) fmt

# 存量 published_article 一次性回填进 webook-search 的 ES 索引（幂等，可重复跑）。
# 需 MySQL（源库）+ etcd（发现）+ webook-search / webook-tag gRPC 在跑；配置同 core（APP_ENV 指某份 yaml）。
# 完成后用 `make -f mk/es.mk count` 复核 ES 实际文档数（search.Index 对 ES 写失败静默降级）。
BACKFILL_ENV ?= config/local.yaml
backfill-search:
	@cd internal && APP_ENV=$(BACKFILL_ENV) go run ./cmd/backfill-search

help:
	@echo "make build              - 编译 Linux 二进制（./webook）"
	@echo "make dc                 - 重打 webook 镜像 + docker compose 重启 webook"
	@echo "make up                 - 一键：build + dc（最常用，改完代码运行这条）"
	@echo "make down               - 停 webook 服务（中间件不动）"
	@echo "make fmt                - goimports 格式化（本地包分组）"
	@echo "make fmt-check          - 只检查不改动（CI 用）"
	@echo "make verify             - 提交前全量校验（格式 + workspace + GOWORK=off 构建，抓 replace 错配）"
	@echo "make wire               - 重新生成 wire_gen.go + 格式化"
	@echo "make backfill-search    - 存量文章回填进 search ES 索引（幂等；BACKFILL_ENV 覆盖 yaml）"
	@echo "make -f mk/k8s.mk help  - K3s 部署相关命令"
	@echo ""
	@echo "前置依赖：../deploy/.env.local 必须存在（首次：cp ../deploy/.env.local.example ../deploy/.env.local）"
	@echo "起全套（含中间件）：cd ../deploy && ./deploy.sh local"
