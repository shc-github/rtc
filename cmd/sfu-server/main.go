package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/stan/webrtc/pkg/sfu"
)

var (
	port       = flag.String("port", "8081", "SFU server port")
	signalAddr = flag.String("signal", "localhost:8080", "Signal server address for integration")
)

func main() {
	flag.Parse()

	// 创建 SFU 服务器
	sfuServer := sfu.NewServer([]webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
		{URLs: []string{"stun:stun1.l.google.com:19302"}},
	})

	// WebSocket 升级器
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// 处理 WebSocket 连接
	http.HandleFunc("/sfu", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		// 从 URL 参数获取用户信息
		userID := r.URL.Query().Get("user_id")
		roomID := r.URL.Query().Get("room_id")

		if userID == "" || roomID == "" {
			log.Printf("Missing user_id or room_id")
			conn.Close()
			return
		}

		sfuServer.HandleWebSocket(conn, userID, roomID)
	})

	// 健康检查
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 房间状态
	http.HandleFunc("/rooms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rooms := sfuServer.GetRoomIDs()
		w.Write([]byte("{\"rooms\": \"" + strconv.Itoa(len(rooms)) + "\"}"))
	})

	log.Printf("SFU server starting on %s", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf("SFU server error: %v", err)
	}
}