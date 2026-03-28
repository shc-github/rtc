# 构建阶段
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 安装依赖
RUN apk add --no-cache git

# 设置 Go 代理
ENV GOPROXY=https://goproxy.cn,direct

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=0 GOOS=linux go build -o /signal-server cmd/signal-server/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /sfu-server cmd/sfu-server/main.go

# 运行阶段
FROM alpine:latest

WORKDIR /app

# 安装 ca-certificates (HTTPS 需要)
RUN apk --no-cache add ca-certificates

# 从构建阶段复制二进制文件
COPY --from=builder /signal-server /app/signal-server
COPY --from=builder /sfu-server /app/sfu-server

# 复制 web 静态文件
COPY web /app/web

# 暴露端口
EXPOSE 8080 8081

# 启动脚本
COPY docker-entrypoint.sh /app/
RUN chmod +x /app/docker-entrypoint.sh

ENTRYPOINT ["/app/docker-entrypoint.sh"]