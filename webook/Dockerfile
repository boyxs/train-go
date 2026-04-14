# 基础镜像
FROM alpine:3.20

# 安装ca证书
# RUN apk add --no-cache ca-certificates

COPY webook /app/webook
COPY config /app/config

WORKDIR /app

RUN chmod +x /app/webook
CMD ["/app/webook"]
