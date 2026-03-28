package signal

import (
	"sync"

	"github.com/gorilla/websocket"
)

// User 表示房间中的一个用户
type User struct {
	ID       string
	Nickname string
	Conn     *websocket.Conn
	RoomID   string
}

// Room 表示一个会话房间
type Room struct {
	ID      string
	Users   map[string]*User
	mutex   sync.RWMutex
	onJoin  func(roomID, userID string)
	onLeave func(roomID, userID string)
}

// NewRoom 创建新房间
func NewRoom(id string) *Room {
	return &Room{
		ID:    id,
		Users: make(map[string]*User),
	}
}

// OnJoin 设置用户加入回调
func (r *Room) OnJoin(callback func(roomID, userID string)) {
	r.onJoin = callback
}

// OnLeave 设置用户离开回调
func (r *Room) OnLeave(callback func(roomID, userID string)) {
	r.onLeave = callback
}

// AddUser 添加用户到房间
func (r *Room) AddUser(user *User) {
	r.mutex.Lock()
	r.Users[user.ID] = user
	r.mutex.Unlock()

	if r.onJoin != nil {
		r.onJoin(r.ID, user.ID)
	}
}

// RemoveUser 从房间移除用户
func (r *Room) RemoveUser(userID string) {
	r.mutex.Lock()
	delete(r.Users, userID)
	r.mutex.Unlock()

	if r.onLeave != nil {
		r.onLeave(r.ID, userID)
	}
}

// GetUser 获取用户
func (r *Room) GetUser(userID string) *User {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.Users[userID]
}

// GetUserIDs 获取所有用户ID
func (r *Room) GetUserIDs() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	ids := make([]string, 0, len(r.Users))
	for id := range r.Users {
		ids = append(ids, id)
	}
	return ids
}

// UserCount 获取用户数量
func (r *Room) UserCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.Users)
}

// Broadcast 向房间内所有用户广播消息（排除发送者）
func (r *Room) Broadcast(msg *Message, excludeUserID string) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for id, user := range r.Users {
		if id == excludeUserID {
			continue
		}
		if err := user.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return err
		}
	}
	return nil
}

// BroadcastAll 向房间内所有用户广播消息
func (r *Room) BroadcastAll(msg *Message) error {
	return r.Broadcast(msg, "")
}

// SendTo 向指定用户发送消息
func (r *Room) SendTo(userID string, msg *Message) error {
	r.mutex.RLock()
	user, ok := r.Users[userID]
	r.mutex.RUnlock()

	if !ok {
		return nil // 用户不存在，静默处理
	}

	data, err := msg.Encode()
	if err != nil {
		return err
	}

	return user.Conn.WriteMessage(websocket.TextMessage, data)
}

// RoomManager 房间管理器
type RoomManager struct {
	rooms  map[string]*Room
	mutex  sync.RWMutex
	onRoomCreated func(roomID string)
	onRoomDeleted func(roomID string)
}

// NewRoomManager 创建房间管理器
func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*Room),
	}
}

// OnRoomCreated 设置房间创建回调
func (rm *RoomManager) OnRoomCreated(callback func(roomID string)) {
	rm.onRoomCreated = callback
}

// OnRoomDeleted 设置房间删除回调
func (rm *RoomManager) OnRoomDeleted(callback func(roomID string)) {
	rm.onRoomDeleted = callback
}

// CreateRoom 创建房间
func (rm *RoomManager) CreateRoom(id string) *Room {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if room, ok := rm.rooms[id]; ok {
		return room
	}

	room := NewRoom(id)
	rm.rooms[id] = room

	if rm.onRoomCreated != nil {
		rm.onRoomCreated(id)
	}

	return room
}

// GetRoom 获取房间
func (rm *RoomManager) GetRoom(id string) *Room {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	return rm.rooms[id]
}

// DeleteRoom 删除房间
func (rm *RoomManager) DeleteRoom(id string) {
	rm.mutex.Lock()
	delete(rm.rooms, id)
	rm.mutex.Unlock()

	if rm.onRoomDeleted != nil {
		rm.onRoomDeleted(id)
	}
}

// GetRoomIDs 获取所有房间ID
func (rm *RoomManager) GetRoomIDs() []string {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	ids := make([]string, 0, len(rm.rooms))
	for id := range rm.rooms {
		ids = append(ids, id)
	}
	return ids
}

// RoomCount 获取房间数量
func (rm *RoomManager) RoomCount() int {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	return len(rm.rooms)
}

// GetOrCreateRoom 获取或创建房间
func (rm *RoomManager) GetOrCreateRoom(id string) *Room {
	room := rm.GetRoom(id)
	if room == nil {
		room = rm.CreateRoom(id)
	}
	return room
}

// CleanupEmptyRooms 清理空房间
func (rm *RoomManager) CleanupEmptyRooms() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for id, room := range rm.rooms {
		if room.UserCount() == 0 {
			delete(rm.rooms, id)
			if rm.onRoomDeleted != nil {
				rm.onRoomDeleted(id)
			}
		}
	}
}