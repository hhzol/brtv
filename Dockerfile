FROM golang:1.21-alpine AS builder

WORKDIR /build

COPY main.go .
COPY go.mod go.sum* ./

RUN go mod tidy || true && \
    CGO_ENABLED=0 GOOS=linux go build -o brtv .

FROM alpine:3.18

WORKDIR /app

COPY --from=builder /build/brtv .

EXPOSE 8080

CMD ["./brtv"]
