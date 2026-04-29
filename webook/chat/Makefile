# ── chat 服务本地命令 ──────────────────────────────────────
# 在 webook/chat/ 目录下运行 make <target>

.PHONY: build run vet test

build:
	@go build -o ../bin/chat .

run:
	@go run .

vet:
	@go vet ./...

test:
	@go test ./...
