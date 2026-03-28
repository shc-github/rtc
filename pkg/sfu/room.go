package sfu

import (
	"io"
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Room 表示 SFU 中的一个媒体房间
type Room struct {
	ID          string
	Peers       map[string]*Peer // userID -> Peer
	TrackRouter *TrackRouter
	mutex       sync.RWMutex
}

// NewRoom 创建新的 SFU 房间
func NewRoom(id string) *Room {
	return &Room{
		ID:          id,
		Peers:       make(map[string]*Peer),
		TrackRouter: NewTrackRouter(),
	}
}

// AddPeer 添加 Peer 到房间
func (r *Room) AddPeer(peer *Peer) {
	r.mutex.Lock()
	r.Peers[peer.ID] = peer
	r.mutex.Unlock()

	// 设置轨道接收回调
	peer.Connection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[SFU] Room %s: Peer %s received track %s", r.ID, peer.ID, track.ID())
		peer.AddRemoteTrack(track)
		r.handleIncomingTrack(peer, track)
	})

	log.Printf("[SFU] Room %s: Peer %s joined", r.ID, peer.ID)
}

// RemovePeer 从房间移除 Peer
func (r *Room) RemovePeer(peerID string) {
	r.mutex.Lock()
	peer, ok := r.Peers[peerID]
	if ok {
		peer.Close()
		delete(r.Peers, peerID)
	}
	r.mutex.Unlock()

	if ok {
		// 移除该 Peer 的所有路由
		r.TrackRouter.RemoveAllTracks(peerID)
		log.Printf("[SFU] Room %s: Peer %s left", r.ID, peerID)
	}
}

// GetPeer 获取 Peer
func (r *Room) GetPeer(peerID string) *Peer {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.Peers[peerID]
}

// GetPeerIDs 获取所有 Peer ID
func (r *Room) GetPeerIDs() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	ids := make([]string, 0, len(r.Peers))
	for id := range r.Peers {
		ids = append(ids, id)
	}
	return ids
}

// PeerCount 获取 Peer 数量
func (r *Room) PeerCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.Peers)
}

// handleIncomingTrack 处理接收到的轨道
func (r *Room) handleIncomingTrack(sourcePeer *Peer, remoteTrack *webrtc.TrackRemote) {
	// 为此轨道创建本地轨道用于转发
	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		remoteTrack.Codec().RTPCodecCapability,
		remoteTrack.ID(),
		remoteTrack.StreamID(),
	)
	if err != nil {
		log.Printf("[SFU] Failed to create local track: %v", err)
		return
	}

	// 将本地轨道添加到路由器
	r.TrackRouter.AddTrack(sourcePeer.ID, remoteTrack.ID(), localTrack)

	// 启动转发
	go r.forwardTrack(sourcePeer.ID, remoteTrack, localTrack)

	// 将此轨道添加到房间内其他 Peer（发送 new-track 通知）
	r.addTrackToOtherPeers(sourcePeer.ID, localTrack)
}

// forwardTrack 转发轨道数据
func (r *Room) forwardTrack(sourcePeerID string, remoteTrack *webrtc.TrackRemote, localTrack *webrtc.TrackLocalStaticRTP) {
	for {
		rtpPacket, _, err := remoteTrack.ReadRTP()
		if err != nil {
			if err == io.EOF {
				log.Printf("[SFU] Track %s from %s ended", remoteTrack.ID(), sourcePeerID)
				return
			}
			log.Printf("[SFU] Track read error: %v", err)
			continue
		}

		if err := localTrack.WriteRTP(rtpPacket); err != nil {
			log.Printf("[SFU] Track write error: %v", err)
		}
	}
}

// addTrackToOtherPeers 将轨道添加到其他 Peer
func (r *Room) addTrackToOtherPeers(sourcePeerID string, localTrack *webrtc.TrackLocalStaticRTP) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for peerID, peer := range r.Peers {
		if peerID == sourcePeerID {
			continue
		}

		// 添加轨道到 PeerConnection
		_, err := peer.Connection.AddTrack(localTrack)
		if err != nil {
			log.Printf("[SFU] Failed to add track to peer %s: %v", peerID, err)
			continue
		}

		log.Printf("[SFU] Added track %s to peer %s", localTrack.ID(), peerID)
	}
}

// GetExistingTracks 获取房间内现有的轨道（用于新加入的 Peer）
func (r *Room) GetExistingTracks() []*webrtc.TrackLocalStaticRTP {
	return r.TrackRouter.GetAllTracks()
}