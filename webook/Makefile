IMAGE := train/webook:1.0.0
# 从 go.mod 第一行提取模块路径，单一真相源
MODULE := $(shell head -1 go.mod | awk '{print $$2}')

.PHONY: build dc up fmt fmt-check help

# 宿主机编译 Linux 二进制（产物：./webook）
build:
	@rm -f webook
	@go mod tidy
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=k8s -o webook .

# docker compose 构建镜像 + 启动（需先 make build）
dc:
	@docker rmi -f $(IMAGE) || true
	@docker compose build webook
	@docker compose up -d webook

# 一键：编译 + 构建镜像 + 启动
up: build dc

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

help:
	@echo "make build              - 编译 Linux 二进制（./webook）"
	@echo "make dc                 - 打镜像 + docker compose 启动 webook"
	@echo "make up                 - 一键：build + dc"
	@echo "make fmt                - goimports 格式化（本地包分组）"
	@echo "make fmt-check          - 只检查不改动（CI 用）"
	@echo "make -f mk/k8s.mk help  - K3s 部署相关命令"
