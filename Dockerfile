# 1. 编译阶段
FROM golang:1.21-alpine AS builder

# 预先安装证书、时区文件，并预建数据目录
RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /build/data

WORKDIR /build

# 优先复制依赖配置文件，提升 Docker 构建缓存利用率
COPY go.mod go.sum* ./
RUN go mod download

COPY main.go .

# -ldflags="-s -w": 剥离符号表与调试信息，极大缩减二进制体积
# -trimpath: 移除代码绝对路径元数据
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o brtv .

# 2. 最终运行阶段 (完全空白镜像 scratch)
FROM scratch

# 从 builder 拷贝 SSL 证书、时区文件及预建好的 data 目录
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /build/data /app/data

WORKDIR /app

# 复制 Go 编译产物及相关配置文件
COPY --from=builder /build/brtv .
COPY cookies.json playlist.txt channels.txt cookies.html ./
COPY help/ ./help/

EXPOSE 6600

CMD ["./brtv"]