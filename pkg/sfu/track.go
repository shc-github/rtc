package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

// TrackRouter 轨道路由器，管理轨道的来源和转发
type TrackRouter struct {
	// tracks: sourcePeerID -> trackID -> localTrack
	tracks map[string]map[string]*webrtc.TrackLocalStaticRTP
	mutex  sync.RWMutex
}

// NewTrackRouter 创建轨道路由器
func NewTrackRouter() *TrackRouter {
	return &TrackRouter{
		tracks: make(map[string]map[string]*webrtc.TrackLocalStaticRTP),
	}
}

// AddTrack 添加轨道
func (r *TrackRouter) AddTrack(sourcePeerID, trackID string, localTrack *webrtc.TrackLocalStaticRTP) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.tracks[sourcePeerID] == nil {
		r.tracks[sourcePeerID] = make(map[string]*webrtc.TrackLocalStaticRTP)
	}
	r.tracks[sourcePeerID][trackID] = localTrack
}

// RemoveTrack 移除轨道
func (r *TrackRouter) RemoveTrack(sourcePeerID, trackID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.tracks[sourcePeerID] != nil {
		delete(r.tracks[sourcePeerID], trackID)
	}
}

// RemoveAllTracks 移除某 Peer 的所有轨道
func (r *TrackRouter) RemoveAllTracks(sourcePeerID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.tracks, sourcePeerID)
}

// GetTrack 获取轨道
func (r *TrackRouter) GetTrack(sourcePeerID, trackID string) *webrtc.TrackLocalStaticRTP {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if r.tracks[sourcePeerID] == nil {
		return nil
	}
	return r.tracks[sourcePeerID][trackID]
}

// GetAllTracks 获取所有轨道
func (r *TrackRouter) GetAllTracks() []*webrtc.TrackLocalStaticRTP {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	allTracks := make([]*webrtc.TrackLocalStaticRTP, 0)
	for _, peerTracks := range r.tracks {
		for _, track := range peerTracks {
			allTracks = append(allTracks, track)
		}
	}
	return allTracks
}

// GetTracksFromPeer 获取某 Peer 的所有轨道
func (r *TrackRouter) GetTracksFromPeer(sourcePeerID string) []*webrtc.TrackLocalStaticRTP {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peerTracks := r.tracks[sourcePeerID]
	if peerTracks == nil {
		return nil
	}

	tracks := make([]*webrtc.TrackLocalStaticRTP, 0, len(peerTracks))
	for _, track := range peerTracks {
		tracks = append(tracks, track)
	}
	return tracks
}

// TrackCount 获取轨道总数
func (r *TrackRouter) TrackCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	count := 0
	for _, peerTracks := range r.tracks {
		count += len(peerTracks)
	}
	return count
}