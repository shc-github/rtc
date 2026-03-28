#!/bin/sh
set -e

# 获取公网地址或使用环境变量
PUBLIC_HOST=${PUBLIC_HOST:-localhost}
SIGNAL_PORT=${SIGNAL_PORT:-8080}
SFU_PORT=${SFU_PORT:-8081}

# SFU URL (客户端访问)
SFU_PUBLIC_URL=${SFU_PUBLIC_URL:-ws://${PUBLIC_HOST}:${SFU_PORT}/sfu}

echo "Starting WebRTC server..."
echo "Public Host: ${PUBLIC_HOST}"
echo "Signal Port: ${SIGNAL_PORT}"
echo "SFU Port: ${SFU_PORT}"
echo "SFU Public URL: ${SFU_PUBLIC_URL}"

# 启动 SFU 服务器 (后台运行)
/app/sfu-server -port ${SFU_PORT} &

# 等待 SFU 启动
sleep 1

# 启动信令服务器 (前台运行)
exec /app/signal-server \
    -addr :${SIGNAL_PORT} \
    -sfu ${SFU_PUBLIC_URL} \
    -sfu-threshold ${SFU_THRESHOLD:-3}