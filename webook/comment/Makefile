# ── comment 服务本地命令 ──────────────────────────────────────
# 在 webook/comment/ 目录下运行 make <target>

.PHONY: build run vet test

build:
	@go build -o ../bin/comment .

run:
	@go run .

vet:
	@go vet ./...

test:
	@go test ./...
