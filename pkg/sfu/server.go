package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// Server SFU 服务器
type Server struct {
	rooms      map[string]*Room
	config     webrtc.Configuration
	peers      map[string]*Peer      // userID -> Peer
	userRooms  map[string]string     // userID -> roomID
	connections map[string]*websocket.Conn // userID -> WebSocket connection
	mutex      sync.RWMutex
}

// NewServer 创建 SFU 服务器
func NewServer(iceServers []webrtc.ICEServer) *Server {
	return &Server{
		rooms:       make(map[string]*Room),
		peers:       make(map[string]*Peer),
		userRooms:   make(map[string]string),
		connections: make(map[string]*websocket.Conn),
		config: webrtc.Configuration{
			ICEServers: iceServers,
		},
	}
}

// GetOrCreateRoom 获取或创建房间
func (s *Server) GetOrCreateRoom(roomID string) *Room {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		room = NewRoom(roomID)
		s.rooms[roomID] = room
		log.Printf("[SFU] Created room: %s", roomID)
	}
	return room
}

// GetRoom 获取房间
func (s *Server) GetRoom(roomID string) *Room {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.rooms[roomID]
}

// RemoveRoom 移除房间
func (s *Server) RemoveRoom(roomID string) {
	s.mutex.Lock()
	delete(s.rooms, roomID)
	s.mutex.Unlock()
	log.Printf("[SFU] Removed room: %s", roomID)
}

// CreatePeer 为用户创建 PeerConnection
func (s *Server) CreatePeer(userID, roomID string) (*Peer, error) {
	room := s.GetOrCreateRoom(roomID)

	peer, err := NewPeer(userID, roomID, s.config)
	if err != nil {
		return nil, err
	}

	// 添加房间内现有轨道到新 Peer
	existingTracks := room.GetExistingTracks()
	for _, track := range existingTracks {
		_, err := peer.Connection.AddTrack(track)
		if err != nil {
			log.Printf("[SFU] Failed to add existing track to peer %s: %v", userID, err)
		} else {
			peer.AddLocalTrack(track.ID(), track)
			log.Printf("[SFU] Added existing track %s to new peer %s", track.ID(), userID)
		}
	}

	// 添加 Peer 到房间
	room.AddPeer(peer)

	// 记录用户信息
	s.mutex.Lock()
	s.peers[userID] = peer
	s.userRooms[userID] = roomID
	s.mutex.Unlock()

	return peer, nil
}

// RemovePeer 移除 Peer
func (s *Server) RemovePeer(userID string) {
	s.mutex.Lock()
	peer, ok := s.peers[userID]
	roomID := s.userRooms[userID]
	delete(s.peers, userID)
	delete(s.userRooms, roomID)
	delete(s.connections, userID)
	s.mutex.Unlock()

	if ok && peer != nil {
		room := s.GetRoom(roomID)
		if room != nil {
			room.RemovePeer(userID)

			// 如果房间为空，移除房间
			if room.PeerCount() == 0 {
				s.RemoveRoom(roomID)
			}
		}
	}

	log.Printf("[SFU] Removed peer: %s", userID)
}

// GetPeer 获取 Peer
func (s *Server) GetPeer(userID string) *Peer {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.peers[userID]
}

// HandleWebSocket 处理 WebSocket 连接
func (s *Server) HandleWebSocket(conn *websocket.Conn, userID, roomID string) {
	s.mutex.Lock()
	s.connections[userID] = conn
	s.mutex.Unlock()

	log.Printf("[SFU] WebSocket connected for user %s in room %s", userID, roomID)

	// 创建 PeerConnection
	peer, err := s.CreatePeer(userID, roomID)
	if err != nil {
		log.Printf("[SFU] Failed to create peer: %v", err)
		s.sendError(conn, "Failed to create peer connection")
		return
	}

	// 监听 ICE candidate
	peer.Connection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		s.sendCandidate(conn, candidate.ToJSON())
	})

	// 发送房间内其他用户列表
	room := s.GetRoom(roomID)
	if room != nil {
		otherPeers := room.GetPeerIDs()
		peersPayload := make([]string, 0)
		for _, pid := range otherPeers {
			if pid != userID {
				peersPayload = append(peersPayload, pid)
			}
		}
		s.sendPeersList(conn, peersPayload)
	}

	// 消息处理循环
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[SFU] WebSocket read error: %v", err)
			}
			break
		}

		s.handleMessage(conn, peer, data)
	}

	// 清理
	s.RemovePeer(userID)
	conn.Close()
}

// handleMessage 处理消息
func (s *Server) handleMessage(conn *websocket.Conn, peer *Peer, data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[SFU] Message decode error: %v", err)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "offer":
		s.handleOffer(conn, peer, msg)
	case "candidate":
		s.handleCandidate(peer, msg)
	default:
		log.Printf("[SFU] Unknown message type: %s", msgType)
	}
}

// handleOffer 处理 SDP Offer
func (s *Server) handleOffer(conn *websocket.Conn, peer *Peer, msg map[string]interface{}) {
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		return
	}

	sdpStr, ok := payload["sdp"].(string)
	if !ok {
		return
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpStr,
	}

	if err := peer.Connection.SetRemoteDescription(offer); err != nil {
		log.Printf("[SFU] Set remote description error: %v", err)
		s.sendError(conn, "Failed to set remote description")
		return
	}

	// 创建 Answer
	answer, err := peer.Connection.CreateAnswer(nil)
	if err != nil {
		log.Printf("[SFU] Create answer error: %v", err)
		s.sendError(conn, "Failed to create answer")
		return
	}

	if err := peer.Connection.SetLocalDescription(answer); err != nil {
		log.Printf("[SFU] Set local description error: %v", err)
		return
	}

	s.sendAnswer(conn, answer)
	log.Printf("[SFU] Sent answer to peer %s", peer.ID)
}

// handleCandidate 处理 ICE Candidate
func (s *Server) handleCandidate(peer *Peer, msg map[string]interface{}) {
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		return
	}

	candidateStr, _ := payload["candidate"].(string)
	sdpMid, _ := payload["sdp_mid"].(string)
	sdpMLineIndex, _ := payload["sdp_m_line_index"].(float64)

	mLineIndex := uint16(int(sdpMLineIndex))
	candidate := webrtc.ICECandidateInit{
		Candidate:     candidateStr,
		SDPMid:        &sdpMid,
		SDPMLineIndex: &mLineIndex,
	}

	if err := peer.Connection.AddICECandidate(candidate); err != nil {
		log.Printf("[SFU] Add ICE candidate error: %v", err)
	}
}

// sendAnswer 发送 SDP Answer
func (s *Server) sendAnswer(conn *websocket.Conn, answer webrtc.SessionDescription) {
	msg := map[string]interface{}{
		"type": "answer",
		"payload": map[string]string{
			"sdp":  answer.SDP,
			"type": "answer",
		},
		"timestamp": json.Number("0"),
	}
	conn.WriteJSON(msg)
}

// sendCandidate 发送 ICE Candidate
func (s *Server) sendCandidate(conn *websocket.Conn, candidate webrtc.ICECandidateInit) {
	var mLineIndex int
	if candidate.SDPMLineIndex != nil {
		mLineIndex = int(*candidate.SDPMLineIndex)
	}
	var sdpMid string
	if candidate.SDPMid != nil {
		sdpMid = *candidate.SDPMid
	}

	msg := map[string]interface{}{
		"type": "candidate",
		"payload": map[string]interface{}{
			"candidate":        candidate.Candidate,
			"sdp_mid":          sdpMid,
			"sdp_m_line_index": mLineIndex,
		},
		"timestamp": json.Number("0"),
	}
	conn.WriteJSON(msg)
}

// sendPeersList 发送用户列表
func (s *Server) sendPeersList(conn *websocket.Conn, peers []string) {
	msg := map[string]interface{}{
		"type": "peers",
		"payload": map[string]interface{}{
			"peers": peers,
		},
		"timestamp": json.Number("0"),
	}
	conn.WriteJSON(msg)
}

// sendError 发送错误消息
func (s *Server) sendError(conn *websocket.Conn, message string) {
	msg := map[string]interface{}{
		"type": "error",
		"payload": map[string]interface{}{
			"message": message,
		},
		"timestamp": json.Number("0"),
	}
	conn.WriteJSON(msg)
}

// GetRoomIDs 获取所有房间 ID
func (s *Server) GetRoomIDs() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	ids := make([]string, 0, len(s.rooms))
	for id := range s.rooms {
		ids = append(ids, id)
	}
	return ids
}

// RoomCount 获取房间数量
func (s *Server) RoomCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.rooms)
}