FROM golang:1.21-alpine AS builder

WORKDIR /build

COPY main.go .
COPY go.mod go.sum* ./

RUN go mod tidy || true && \
    CGO_ENABLED=0 GOOS=linux go build -o brtv .

FROM alpine:3.18

WORKDIR /app

COPY --from=builder /build/brtv .
COPY cookies.json .
COPY playlist.txt .
COPY channels.txt .

RUN mkdir -p /app/config && \
    cp cookies.json /app/config/cookies.json

EXPOSE 6600

CMD ["./brtv"]
