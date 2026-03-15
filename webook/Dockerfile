# 基础镜像
FROM ubuntu:20.04

COPY webook /app/webook
COPY config /app/config

WORKDIR /app

RUN chmod +x /app/webook
CMD ["/app/webook"]
