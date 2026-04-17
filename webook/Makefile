IMAGE := train/webook:1.0.0

.PHONY: build dc up help

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

help:
	@echo "make build              - 编译 Linux 二进制（./webook）"
	@echo "make dc                 - 打镜像 + docker compose 启动 webook"
	@echo "make up                 - 一键：build + dc"
	@echo "make -f mk/k8s.mk help  - K3s 部署相关命令"
