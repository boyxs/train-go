# ── 多阶段构建：build context = webook/ 仓根
# 用法：
#   CI / 服务器：cd webook && docker build -f internal/Dockerfile -t webook:tag .
#   compose local：build.context=../webook, dockerfile=internal/Dockerfile

FROM golang:1.25.6-alpine AS builder

ARG VERSION=dev
WORKDIR /src

COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -tags=k8s \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/webook ./internal

# ── Runtime stage
FROM alpine:3.20

ARG VERSION=dev
ENV WEBOOK_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}

COPY --from=builder /out/webook /app/webook
COPY internal/config /app/config

WORKDIR /app

# 默认加载 prod 配置；docker compose / K8s 通过 APP_ENV 覆盖
ENV APP_ENV=config/prod.yaml

CMD ["/app/webook"]
