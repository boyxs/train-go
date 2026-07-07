# ── 多阶段构建：build context = webook/ 仓根（relation 依赖 webook/pkg、webook/api）
# 用法：
#   CI / 服务器：cd webook && docker build -f relation/Dockerfile -t webook-relation:tag .
#   compose local：build.context=../webook, dockerfile=relation/Dockerfile

FROM golang:1.25.6-alpine AS builder

ARG VERSION=dev
WORKDIR /src

# 先 mod download，利用 docker layer cache
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

# 整仓 COPY，go build 能解析所有 webook/pkg、webook/api/gen 等子包
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/relation ./relation

# ── Runtime stage
FROM alpine:3.20

ARG VERSION=dev
ENV RELATION_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}

COPY --from=builder /out/relation /app/relation
COPY relation/config /app/config

WORKDIR /app

# 默认加载 prod 配置；docker compose / K8s 通过 APP_ENV 覆盖
ENV APP_ENV=config/prod.yaml

CMD ["/app/relation"]
