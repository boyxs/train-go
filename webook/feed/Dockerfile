# ── 多阶段构建：build context = webook/ 仓根（feed 依赖 webook/pkg、webook/api、webook/shared）
# 用法：
#   CI / 服务器：cd webook && docker build -f feed/Dockerfile -t webook-feed:tag .
#   compose local：build.context=../webook, dockerfile=feed/Dockerfile

FROM golang:1.25.6-alpine AS builder

ARG VERSION=dev
WORKDIR /src
# 多模块 + go.work：GOWORK=off 用各 go.mod 的 replace 解析本地依赖（确定性，不依赖 workspace）
ENV GOFLAGS=-mod=readonly GOWORK=off GOPROXY=https://goproxy.cn,direct

# 缓存层：先 COPY feed 及本地依赖的 go.mod/go.sum（shared 无外部依赖，仅 go.mod），再 download
COPY feed/go.mod feed/go.sum ./feed/
COPY api/go.mod  api/go.sum  ./api/
COPY pkg/go.mod  pkg/go.sum  ./pkg/
COPY shared/go.mod           ./shared/
RUN cd feed && go mod download

# 再 COPY 源码，go build 经 feed/go.mod 的 replace 解析 ../api ../pkg ../shared
COPY api/    ./api/
COPY pkg/    ./pkg/
COPY shared/ ./shared/
COPY feed/ ./feed/

RUN cd feed && CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/feed .

# ── Runtime stage
FROM alpine:3.20

ARG VERSION=dev
ENV FEED_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}

COPY --from=builder /out/feed /app/feed
COPY feed/config /app/config

WORKDIR /app

# 默认加载 prod 配置；docker compose / K8s 通过 APP_ENV 覆盖
ENV APP_ENV=config/prod.yaml

CMD ["/app/feed"]
