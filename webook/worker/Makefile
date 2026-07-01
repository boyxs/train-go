# ── worker 调度器本地命令 ──────────────────────────────────────
# 在 webook/worker/ 目录下运行 make <target>

.PHONY: build run vet test

build:
	@go build -o ../bin/worker .

run:
	@go run .

vet:
	@go vet ./...

test:
	@go test ./...
