# ── 编译阶段 ──
FROM golang:1.25-alpine AS builder
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
# 单独 COPY go.mod/go.sum 利用 Docker 层缓存，依赖不变时跳过下载
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=k8s -o webook .

# ── 运行阶段 ──
FROM alpine:3.20

COPY --from=builder /src/webook /app/webook
COPY --from=builder /src/config /app/config

WORKDIR /app

RUN chmod +x /app/webook
CMD ["/app/webook"]
