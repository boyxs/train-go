# ── 多阶段构建：build context = webook/ 仓根（worker 依赖 webook/pkg、webook/api）
# 用法：
#   CI / 服务器：cd webook && docker build -f worker/Dockerfile -t webook-worker:tag .
#   compose local：build.context=../webook, dockerfile=worker/Dockerfile

FROM golang:1.25.6-alpine AS builder

ARG VERSION=dev
WORKDIR /src

COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/worker ./worker

# ── Runtime stage
FROM alpine:3.20

ARG VERSION=dev
ENV WORKER_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}

COPY --from=builder /out/worker /app/worker
COPY worker/config /app/config

WORKDIR /app

# 默认加载 prod 配置；docker compose / K8s 通过 APP_ENV 覆盖
ENV APP_ENV=config/prod.yaml

CMD ["/app/worker"]
