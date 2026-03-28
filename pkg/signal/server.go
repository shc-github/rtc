package signal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Server 信令服务器
type Server struct {
	roomManager *RoomManager
	upgrader    websocket.Upgrader
	users       map[string]*User // userID -> User
	userMutex   sync.RWMutex
	addr        string
	sfuURL      string            // SFU 服务器地址
	sfuThreshold int              // SFU 模式阈值（房间人数达到此值切换到 SFU）
}

// NewServer 创建信令服务器
func NewServer(addr string) *Server {
	return &Server{
		roomManager: NewRoomManager(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源，生产环境需要限制
			},
		},
		users:        make(map[string]*User),
		addr:         addr,
		sfuThreshold: 3, // 默认 3 人以上切换 SFU
	}
}

// SetSFUURL 设置 SFU 服务器地址
func (s *Server) SetSFUURL(url string) {
	s.sfuURL = url
}

// SetSFUThreshold 设置 SFU 模式阈值
func (s *Server) SetSFUThreshold(threshold int) {
	s.sfuThreshold = threshold
}

// Start 启动服务器
func (s *Server) Start() error {
	http.HandleFunc("/ws", s.HandleWebSocket)
	http.HandleFunc("/rooms", s.HandleRoomList)
	http.HandleFunc("/js/", s.HandleStatic)
	http.HandleFunc("/css/", s.HandleStatic)
	http.HandleFunc("/", s.HandleIndex)

	log.Printf("Signal server starting on %s", s.addr)
	return http.ListenAndServe(s.addr, nil)
}

// HandleWebSocket 处理 WebSocket 连接
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// 生成用户ID
	userID := uuid.New().String()
	user := &User{
		ID:   userID,
		Conn: conn,
	}

	s.userMutex.Lock()
	s.users[userID] = user
	s.userMutex.Unlock()

	log.Printf("User connected: %s", userID)

	// 发送用户ID给客户端
 welcomeMsg := NewMessage(TypeUserJoined, "", userID).WithPayload(UserPayload{UserID: userID})
	s.sendMessage(conn, welcomeMsg)

	// 处理消息循环
	defer func() {
		s.handleDisconnect(userID)
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		s.handleMessage(user, data)
	}
}

// handleMessage 处理接收到的消息
func (s *Server) handleMessage(user *User, data []byte) {
	msg, err := DecodeMessage(data)
	if err != nil {
		log.Printf("Message decode error: %v", err)
		s.sendError(user.Conn, 400, "Invalid message format")
		return
	}

	switch msg.Type {
	case TypeJoin:
		s.handleJoin(user, msg)
	case TypeLeave:
		s.handleLeave(user)
	case TypeOffer:
		s.handleOffer(user, msg)
	case TypeAnswer:
		s.handleAnswer(user, msg)
	case TypeCandidate:
		s.handleCandidate(user, msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
		s.sendError(user.Conn, 400, fmt.Sprintf("Unknown message type: %s", msg.Type))
	}
}

// handleJoin 处理加入房间
func (s *Server) handleJoin(user *User, msg *Message) {
	roomID := msg.RoomID
	if roomID == "" {
		s.sendError(user.Conn, 400, "Room ID required")
		return
	}

	// 解析 join payload 获取昵称
	var payload JoinPayload
	if msg.Payload != nil {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			payload.UserID = user.ID
		}
	}
	payload.UserID = user.ID

	// 如果用户已在其他房间，先离开
	if user.RoomID != "" {
		s.handleLeave(user)
	}

	// 获取或创建房间
	room := s.roomManager.GetOrCreateRoom(roomID)

	// 检查是否需要切换到 SFU 模式
	userCountBeforeJoin := room.UserCount()
	shouldSwitchToSFU := s.sfuURL != "" && userCountBeforeJoin+1 >= s.sfuThreshold

	// 更新用户信息
	user.RoomID = roomID
	user.Nickname = payload.Nickname

	// 添加用户到房间
	room.AddUser(user)

	log.Printf("User %s joined room %s (count: %d)", user.ID, roomID, room.UserCount())

	// 如果需要切换到 SFU 模式，通知所有用户
	if shouldSwitchToSFU && s.sfuURL != "" {
		log.Printf("Room %s switching to SFU mode (users: %d)", roomID, room.UserCount())
		sfuMsg := NewMessage(TypeSwitchToSFU, roomID, "").WithPayload(SFUPayload{
			SFUURL: s.sfuURL,
			RoomID: roomID,
			Reason: "room_full",
		})
		room.BroadcastAll(sfuMsg)
	} else {
		// P2P 模式：广播用户加入通知给房间内其他用户
		notifyMsg := NewMessage(TypeUserJoined, roomID, user.ID).WithPayload(payload)
		room.Broadcast(notifyMsg, user.ID)
	}

	// 发送房间内现有用户列表给新用户
	existingUsers := room.GetUserIDs()
	roomInfoMsg := NewMessage(TypeRoomInfo, roomID, user.ID).WithPayload(RoomPayload{
		RoomID:    roomID,
		UserCount: room.UserCount(),
		Users:     existingUsers,
		// 告知新用户当前模式
	})
	if shouldSwitchToSFU {
		// 在 room_info 中也告知 SFU 信息
		roomInfoMsg = NewMessage(TypeRoomInfo, roomID, user.ID).WithPayload(map[string]interface{}{
			"room_id":    roomID,
			"user_count": room.UserCount(),
			"users":      existingUsers,
			"sfu_url":    s.sfuURL,
			"sfu_mode":   true,
		})
	}
	s.sendMessage(user.Conn, roomInfoMsg)
}

// handleLeave 处理离开房间
func (s *Server) handleLeave(user *User) {
	if user.RoomID == "" {
		return
	}

	room := s.roomManager.GetRoom(user.RoomID)
	if room != nil {
		room.RemoveUser(user.ID)

		// 广播用户离开通知
		notifyMsg := NewMessage(TypeUserLeft, user.RoomID, user.ID).WithPayload(UserPayload{UserID: user.ID})
		room.BroadcastAll(notifyMsg)

		// 清理空房间
		if room.UserCount() == 0 {
			s.roomManager.DeleteRoom(user.RoomID)
		}
	}

	user.RoomID = ""
	log.Printf("User %s left room", user.ID)
}

// handleOffer 处理 SDP Offer
func (s *Server) handleOffer(user *User, msg *Message) {
	if user.RoomID == "" {
		s.sendError(user.Conn, 400, "Not in a room")
		return
	}

	room := s.roomManager.GetRoom(user.RoomID)
	if room == nil {
		s.sendError(user.Conn, 404, "Room not found")
		return
	}

	// 如果指定了目标用户，只发给目标
	if msg.TargetID != "" {
		offerMsg := NewMessage(TypeOffer, user.RoomID, user.ID).WithTarget(msg.TargetID).WithPayload(msg.Payload)
		room.SendTo(msg.TargetID, offerMsg)
		log.Printf("Offer sent from %s to %s", user.ID, msg.TargetID)
	} else {
		// 广播给房间内其他用户
		offerMsg := NewMessage(TypeOffer, user.RoomID, user.ID).WithPayload(msg.Payload)
		room.Broadcast(offerMsg, user.ID)
		log.Printf("Offer broadcast from %s in room %s", user.ID, user.RoomID)
	}
}

// handleAnswer 处理 SDP Answer
func (s *Server) handleAnswer(user *User, msg *Message) {
	if user.RoomID == "" {
		s.sendError(user.Conn, 400, "Not in a room")
		return
	}

	room := s.roomManager.GetRoom(user.RoomID)
	if room == nil {
		s.sendError(user.Conn, 404, "Room not found")
		return
	}

	// Answer 必须发送给特定的目标用户（Offer 发送者）
	if msg.TargetID == "" {
		s.sendError(user.Conn, 400, "Target ID required for answer")
		return
	}

	answerMsg := NewMessage(TypeAnswer, user.RoomID, user.ID).WithTarget(msg.TargetID).WithPayload(msg.Payload)
	room.SendTo(msg.TargetID, answerMsg)
	log.Printf("Answer sent from %s to %s", user.ID, msg.TargetID)
}

// handleCandidate 处理 ICE Candidate
func (s *Server) handleCandidate(user *User, msg *Message) {
	if user.RoomID == "" {
		s.sendError(user.Conn, 400, "Not in a room")
		return
	}

	room := s.roomManager.GetRoom(user.RoomID)
	if room == nil {
		s.sendError(user.Conn, 404, "Room not found")
		return
	}

	// Candidate 发送给特定目标或广播
	candidateMsg := NewMessage(TypeCandidate, user.RoomID, user.ID).WithPayload(msg.Payload)
	if msg.TargetID != "" {
		candidateMsg.TargetID = msg.TargetID
		room.SendTo(msg.TargetID, candidateMsg)
		log.Printf("Candidate sent from %s to %s", user.ID, msg.TargetID)
	} else {
		room.Broadcast(candidateMsg, user.ID)
		log.Printf("Candidate broadcast from %s in room %s", user.ID, user.RoomID)
	}
}

// handleDisconnect 处理用户断开连接
func (s *Server) handleDisconnect(userID string) {
	s.userMutex.Lock()
	user, ok := s.users[userID]
	if ok {
		delete(s.users, userID)
	}
	s.userMutex.Unlock()

	if !ok {
		return
	}

	// 用户离开房间
	if user.RoomID != "" {
		s.handleLeave(user)
	}

	log.Printf("User disconnected: %s", userID)
}

// sendMessage 发送消息给连接
func (s *Server) sendMessage(conn *websocket.Conn, msg *Message) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// sendError 发送错误消息
func (s *Server) sendError(conn *websocket.Conn, code int, message string) {
	errMsg := NewMessage(TypeError, "", "").WithPayload(ErrorPayload{
		Code:    code,
		Message: message,
	})
	s.sendMessage(conn, errMsg)
}

// HandleRoomList 处理房间列表请求 (HTTP)
func (s *Server) HandleRoomList(w http.ResponseWriter, r *http.Request) {
	rooms := s.roomManager.GetRoomIDs()
	roomInfos := make([]RoomPayload, 0, len(rooms))
	for _, roomID := range rooms {
		room := s.roomManager.GetRoom(roomID)
		if room != nil {
			roomInfos = append(roomInfos, RoomPayload{
				RoomID:    roomID,
				UserCount: room.UserCount(),
				Users:     room.GetUserIDs(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roomInfos)
}

// HandleIndex 处理首页请求
func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "web/index.html")
}

// HandleStatic 处理静态文件请求
func (s *Server) HandleStatic(w http.ResponseWriter, r *http.Request) {
	// 静态文件在 web 目录下
	path := "web" + r.URL.Path

	// 设置正确的 MIME 类型
	switch {
	case len(path) >= 3 && path[len(path)-3:] == ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case len(path) >= 4 && path[len(path)-4:] == ".css":
		w.Header().Set("Content-Type", "text/css")
	case len(path) >= 5 && path[len(path)-5:] == ".html":
		w.Header().Set("Content-Type", "text/html")
	}

	http.ServeFile(w, r, path)
}