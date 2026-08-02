FROM golang:1.21-alpine AS builder

WORKDIR /build

COPY main.go .
COPY go.mod go.sum* ./

RUN go mod tidy || true && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o brtv .

FROM alpine:3.18

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 一次性将所有必要文件（brtv 可执行文件、cookies.json、txt 模板）放到 /app 根目录
COPY --from=builder /build/brtv .
COPY cookies.json .
COPY playlist.txt .
COPY channels.txt .

# 依然建议预先创建好 data 目录，确保程序写证书时不会因目录不存在或权限问题报错
RUN mkdir -p /app/data && chmod -R 755 /app

EXPOSE 6600

CMD ["./brtv"]