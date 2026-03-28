# Go WebRTC 语音会话系统

基于 Go 语言和 Pion WebRTC 实现的语音会话系统，支持 P2P 和 SFU 两种模式。

## 功能特性

- **P2P 模式**：2人通话使用点对点直连，低延迟
- **SFU 模式**：3人及以上自动切换到 SFU 模式，带宽线性增长
- **自动切换**：根据房间人数自动选择最优模式
- **静音功能**：支持一键静音/取消静音
- **跨平台**：支持 Web 浏览器、桌面、移动端
- **信令服务**：完整的 WebSocket 信令服务器
- **NAT 穿透**：支持 STUN/TURN
- **Docker 部署**：支持一键 Docker 部署

## 技术栈

| 组件 | 技术 |
|------|------|
| WebRTC | [pion/webrtc](https://github.com/pion/webrtc) v4 |
| 信令传输 | WebSocket (gorilla/websocket) |
| 音频编解码 | Opus |

## 项目结构

```
webrtc/
├── cmd/
│   ├── signal-server/
│   │   └── main.go              # 信令服务器入口
│   └── sfu-server/
│       └── main.go              # SFU 媒体服务器入口
├── pkg/
│   ├── signal/                  # 信令模块
│   │   ├── message.go           # 消息协议定义
│   │   ├── room.go              # 房间和用户管理
│   │   └── server.go            # WebSocket 信令服务器
│   ├── sfu/                     # SFU 媒体服务器
│   │   ├── server.go            # SFU 核心逻辑
│   │   ├── room.go              # 媒体房间管理
│   │   ├── peer.go              # PeerConnection 封装
│   │   └── track.go             # 音轨路由器
│   └── webrtc/                  # WebRTC 客户端封装
│       ├── peer.go              # PeerConnection 管理
│       ├── media.go             # 媒体流处理
│       └── audio.go             # 音频编解码器
├── web/
│   ├── index.html               # Web 演示页面
│   └── js/
│       └── client.js            # JavaScript 客户端 SDK
├── Dockerfile                   # Docker 构建文件
├── docker-compose.yml           # Docker Compose 配置
├── docker-entrypoint.sh         # Docker 启动脚本
├── go.mod
├── go.sum
└── README.md
```

## 快速开始

### Docker 一键部署（推荐）

```bash
# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

**公网部署配置：**

修改 `docker-compose.yml` 中的环境变量：

```yaml
environment:
  - PUBLIC_HOST=your-server-ip-or-domain.com
  - SIGNAL_PORT=9000
  - SFU_PORT=9001
  - SFU_THRESHOLD=3
```

然后访问 `http://your-server-ip:9000`

### 本地开发

#### 安装依赖

```bash
go mod download
```

#### 方式一：P2P 模式（2人通话）

```bash
# 启动信令服务器
go run cmd/signal-server/main.go

# 访问 http://localhost:9000
```

### 方式二：SFU 模式（多人会议）

```bash
# 终端1：启动信令服务器（配置 SFU 地址）
go run cmd/signal-server/main.go -sfu ws://localhost:9001/sfu -sfu-threshold 3

# 终端2：启动 SFU 服务器
go run cmd/sfu-server/main.go -port 9001

# 访问 http://localhost:9000
```

### 命令行参数

**信令服务器 (signal-server)**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:9000` | 服务器监听地址 |
| `-sfu` | `""` | SFU 服务器 WebSocket URL |
| `-sfu-threshold` | `3` | 切换到 SFU 模式的用户数阈值 |

**SFU 服务器 (sfu-server)**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-port` | `9001` | SFU 服务器端口 |
| `-signal` | `localhost:9000` | 信令服务器地址 |

## 架构说明

### P2P 模式

```
┌──────────┐                    ┌──────────┐
│  客户端A  │◄──── WebRTC ────►│  客户端B  │
└────┬─────┘                    └────┬─────┘
     │                               │
     └───────────┬───────────────────┘
                 │ WebSocket (信令)
                 ▼
         ┌──────────────┐
         │  信令服务器   │
         └──────────────┘
```

### SFU 模式

```
┌──────────┐                    ┌──────────┐
│  客户端A  │◄───┐          ┌───►│  客户端B  │
└──────────┘    │          │    └──────────┘
                │          │
┌──────────┐    │          │    ┌──────────┐
│  客户端C  │◄───┼── SFU ───┼───►│  客户端D  │
└──────────┘    │          │    └──────────┘
                │          │
                ▼          ▼
         ┌──────────────────────┐
         │      SFU 服务器       │
         └──────────────────────┘
```

## 信令协议

### 消息类型

| 类型 | 方向 | 说明 |
|------|------|------|
| `join` | Client → Server | 加入房间 |
| `leave` | Client → Server | 离开房间 |
| `user_joined` | Server → Client | 用户加入通知 |
| `user_left` | Server → Client | 用户离开通知 |
| `offer` | 双向 | SDP Offer |
| `answer` | 双向 | SDP Answer |
| `candidate` | 双向 | ICE Candidate |
| `room_info` | Server → Client | 房间信息 |
| `switch_to_sfu` | Server → Client | 切换到 SFU 模式 |
| `error` | Server → Client | 错误消息 |

### 消息格式

```json
{
  "type": "offer",
  "room_id": "room-123",
  "user_id": "user-abc",
  "target_id": "user-xyz",
  "payload": {
    "sdp": "...",
    "type": "offer"
  },
  "timestamp": 1640000000000
}
```

## API 接口

### HTTP 接口

| 路径 | 方法 | 说明 |
|------|------|------|
| `/` | GET | Web 演示页面 |
| `/ws` | GET | WebSocket 信令连接 |
| `/rooms` | GET | 获取房间列表 |

### SFU 接口

| 路径 | 方法 | 说明 |
|------|------|------|
| `/sfu?user_id=xxx&room_id=xxx` | GET | WebSocket SFU 连接 |
| `/health` | GET | 健康检查 |
| `/rooms` | GET | SFU 房间状态 |

## 开发指南

### 构建

```bash
# 构建所有组件
go build ./...

# 构建信令服务器
go build -o bin/signal-server cmd/signal-server/main.go

# 构建 SFU 服务器
go build -o bin/sfu-server cmd/sfu-server/main.go
```

### 测试

```bash
# 运行测试
go test ./pkg/...
```

## 生产部署

### Docker Compose 部署（推荐）

```bash
# 1. 修改 docker-compose.yml 中的 PUBLIC_HOST
# PUBLIC_HOST=your-server-ip-or-domain.com

# 2. 启动服务
docker-compose up -d

# 3. 查看日志
docker-compose logs -f

# 4. 停止服务
docker-compose down
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PUBLIC_HOST` | `localhost` | 公网地址（服务器IP或域名） |
| `SIGNAL_PORT` | `9000` | 信令服务器端口 |
| `SFU_PORT` | `9001` | SFU 服务器端口 |
| `SFU_THRESHOLD` | `3` | SFU 模式切换阈值 |
| `SFU_PUBLIC_URL` | 自动生成 | SFU WebSocket URL |

### 使用 Nginx 反向代理

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:9000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }

    location /ws {
        proxy_pass http://localhost:9000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 注意事项

1. **HTTPS**：生产环境必须使用 HTTPS，WebRTC 需要 Secure Context
2. **TURN 服务器**：配置 TURN 服务器解决复杂 NAT 环境
3. **带宽**：SFU 服务器需要足够带宽
4. **监控**：添加监控和日志系统

## 常见问题

### Q: 音频无法播放？

A: 浏览器可能阻止了 autoplay。请点击页面后再尝试，或在控制台手动调用 `document.querySelector('audio').play()`。

### Q: 连接失败？

A: 检查：
1. 浏览器是否允许麦克风权限
2. 是否使用 HTTPS（本地开发可用 localhost）
3. 防火墙是否阻止了 UDP 端口

### Q: NAT 穿透失败？

A: 配置 TURN 服务器：

```go
// 在服务器配置中添加
ICEServers: []webrtc.ICEServer{
    {
        URLs: []string{"turn:your-turn-server:3478"},
        Username: "username",
        Credential: "password",
    },
}
```

## 参考资料

- [Pion WebRTC](https://github.com/pion/webrtc)
- [WebRTC API - MDN](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API)
- [WebRTC 标准规范](https://www.w3.org/TR/webrtc/)

## License

MIT License