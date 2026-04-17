FROM alpine:3.20

COPY webook /app/webook
COPY config /app/config

WORKDIR /app

RUN chmod +x /app/webook
CMD ["/app/webook"]
