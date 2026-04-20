FROM alpine:3.20

# CI 构建时通过 --build-arg VERSION=xxx 注入（对应 docker/build-push-action 的 build-args）
ARG VERSION=dev
ENV WEBOOK_VERSION=${VERSION}
LABEL org.opencontainers.image.version=${VERSION}

COPY webook /app/webook
COPY config /app/config

WORKDIR /app

RUN chmod +x /app/webook

# 默认加载 prod 配置；docker compose / K8s 可通过 APP_ENV 环境变量覆盖
ENV APP_ENV=config/prod.yaml

CMD ["/app/webook"]
