package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Peer 表示 SFU 中的一个对等连接
type Peer struct {
	ID           string
	RoomID       string
	Connection   *webrtc.PeerConnection
	LocalTracks  map[string]*webrtc.TrackLocalStaticRTP // trackID -> local track
	RemoteTracks map[string]*webrtc.TrackRemote         // trackID -> remote track
	mutex        sync.RWMutex
}

// NewPeer 创建新的 SFU Peer
func NewPeer(id, roomID string, config webrtc.Configuration) (*Peer, error) {
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	peer := &Peer{
		ID:           id,
		RoomID:       roomID,
		Connection:   pc,
		LocalTracks:  make(map[string]*webrtc.TrackLocalStaticRTP),
		RemoteTracks: make(map[string]*webrtc.TrackRemote),
	}

	// 设置连接状态回调
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[SFU] Peer %s connection state: %s", id, state)
	})

	return peer, nil
}

// AddLocalTrack 添加本地轨道（用于转发给其他 Peer）
func (p *Peer) AddLocalTrack(trackID string, track *webrtc.TrackLocalStaticRTP) {
	p.mutex.Lock()
	p.LocalTracks[trackID] = track
	p.mutex.Unlock()
}

// GetLocalTrack 获取本地轨道
func (p *Peer) GetLocalTrack(trackID string) *webrtc.TrackLocalStaticRTP {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.LocalTracks[trackID]
}

// GetLocalTracks 获取所有本地轨道
func (p *Peer) GetLocalTracks() []*webrtc.TrackLocalStaticRTP {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	tracks := make([]*webrtc.TrackLocalStaticRTP, 0, len(p.LocalTracks))
	for _, track := range p.LocalTracks {
		tracks = append(tracks, track)
	}
	return tracks
}

// AddRemoteTrack 添加远程轨道（从该 Peer 接收）
func (p *Peer) AddRemoteTrack(track *webrtc.TrackRemote) {
	p.mutex.Lock()
	p.RemoteTracks[track.ID()] = track
	p.mutex.Unlock()
}

// Close 关闭 Peer
func (p *Peer) Close() error {
	p.mutex.Lock()
	p.LocalTracks = nil
	p.RemoteTracks = nil
	p.mutex.Unlock()
	return p.Connection.Close()
}