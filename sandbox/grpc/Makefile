# ── 本地 protoc 生成 ────────────────────────────────────────
# 依赖：protoc + protoc-gen-go + protoc-gen-go-grpc
# 用法：make gen

.PHONY: gen clean tidy test

PROTO_DIR := proto
GEN_DIR   := gen

gen:
	@mkdir -p $(GEN_DIR)
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		$(shell find $(PROTO_DIR) -name '*.proto')

clean:
	@rm -rf $(GEN_DIR)

tidy:
	@go mod tidy

test: gen
	@go test ./...
