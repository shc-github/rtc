package signal

import (
	"encoding/json"
	"time"
)

// MessageType 定义信令消息类型
type MessageType string

const (
	// 连接管理
	TypeJoin       MessageType = "join"        // 加入房间
	TypeLeave      MessageType = "leave"       // 离开房间
	TypeUserJoined MessageType = "user_joined" // 用户加入通知
	TypeUserLeft   MessageType = "user_left"   // 用户离开通知

	// SDP 交换
	TypeOffer  MessageType = "offer"  // SDP Offer
	TypeAnswer MessageType = "answer" // SDP Answer

	// ICE 交换
	TypeCandidate MessageType = "candidate" // ICE Candidate

	// 房间管理
	TypeRoomList MessageType = "room_list" // 房间列表
	TypeRoomInfo MessageType = "room_info" // 房间信息

	// SFU 模式
	TypeSwitchToSFU MessageType = "switch_to_sfu" // 切换到 SFU 模式
	TypeSFUReady    MessageType = "sfu_ready"     // SFU 就绪通知

	// 错误
	TypeError MessageType = "error" // 错误消息
)

// Message 信令消息结构
type Message struct {
	Type      MessageType     `json:"type"`
	RoomID    string          `json:"room_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	TargetID  string          `json:"target_id,omitempty"` // 目标用户ID（用于定向消息）
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// NewMessage 创建新消息
func NewMessage(typ MessageType, roomID, userID string) *Message {
	return &Message{
		Type:      typ,
		RoomID:    roomID,
		UserID:    userID,
		Timestamp: time.Now().UnixMilli(),
	}
}

// WithPayload 设置消息 payload
func (m *Message) WithPayload(payload interface{}) *Message {
	data, err := json.Marshal(payload)
	if err != nil {
		return m
	}
	m.Payload = data
	return m
}

// WithTarget 设置目标用户
func (m *Message) WithTarget(targetID string) *Message {
	m.TargetID = targetID
	return m
}

// Encode 编码消息为 JSON
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage 解码 JSON 为消息
func DecodeMessage(data []byte) (*Message, error) {
	msg := &Message{}
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// JoinPayload 加入房间消息的 payload
type JoinPayload struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname,omitempty"`
}

// SDPPayload SDP 消息的 payload
type SDPPayload struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"` // "offer" 或 "answer"
}

// CandidatePayload ICE Candidate 消息的 payload
type CandidatePayload struct {
	Candidate     string `json:"candidate"`
	SdpMid        string `json:"sdp_mid"`
	SdpMLineIndex int    `json:"sdp_m_line_index"`
}

// UserPayload 用户信息 payload
type UserPayload struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname,omitempty"`
}

// RoomPayload 房间信息 payload
type RoomPayload struct {
	RoomID    string   `json:"room_id"`
	UserCount int      `json:"user_count"`
	Users     []string `json:"users,omitempty"`
}

// ErrorPayload 错误消息 payload
type ErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SFUPayload SFU 切换消息 payload
type SFUPayload struct {
	SFUURL  string `json:"sfu_url"`  // SFU 服务器 WebSocket URL
	RoomID  string `json:"room_id"`  // 房间 ID
	UserID  string `json:"user_id"`  // 用户 ID
	Reason  string `json:"reason"`   // 切换原因
}