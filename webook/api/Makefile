# ── proto 生成 ────────────────────────────────────────────
# 依赖：protoc + protoc-gen-go + protoc-gen-go-grpc
# 用法：make gen / make clean

.PHONY: gen clean tidy

PROTO_DIR := proto
GEN_DIR   := gen

gen:
	@mkdir -p $(GEN_DIR)
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		$(shell find $(PROTO_DIR) -name '*.proto')
	@# protoc 输出的 import 顺序不符 goimports，CI 检查会 fail；统一规整一下
	@goimports -local github.com/boyxs/train-go/webook -w $(GEN_DIR)

clean:
	@rm -rf $(GEN_DIR)

tidy:
	@go mod tidy
