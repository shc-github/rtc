package webrtc

import (
	"log"

	"github.com/pion/webrtc/v4"
)

// Config WebRTC 配置
type Config struct {
	ICEServers []webrtc.ICEServer
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
			{
				URLs: []string{"stun:stun1.l.google.com:19302"},
			},
		},
	}
}

// Peer WebRTC PeerConnection 封装
type Peer struct {
	ID             string
	Connection     *webrtc.PeerConnection
	config         webrtc.Configuration
	onTrack        func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)
	onConnectionStateChange func(state webrtc.PeerConnectionState)
	onICECandidate func(candidate webrtc.ICECandidateInit)
	onError        func(err error)
}

// NewPeer 创建新的 Peer
func NewPeer(id string, cfg *Config) (*Peer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	webrtcConfig := webrtc.Configuration{
		ICEServers: cfg.ICEServers,
	}

	pc, err := webrtc.NewPeerConnection(webrtcConfig)
	if err != nil {
		return nil, err
	}

	peer := &Peer{
		ID:         id,
		Connection: pc,
		config:     webrtcConfig,
	}

	// 设置连接状态回调
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Peer %s connection state: %s", id, state)
		if peer.onConnectionStateChange != nil {
			peer.onConnectionStateChange(state)
		}
	})

	// 设置 ICE candidate 回调
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		init := candidate.ToJSON()
		if peer.onICECandidate != nil {
			peer.onICECandidate(init)
		}
	})

	return peer, nil
}

// OnTrack 设置音轨回调
func (p *Peer) OnTrack(callback func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)) {
	p.onTrack = callback
	p.Connection.OnTrack(callback)
}

// OnConnectionStateChange 设置连接状态回调
func (p *Peer) OnConnectionStateChange(callback func(state webrtc.PeerConnectionState)) {
	p.onConnectionStateChange = callback
}

// OnICECandidate 设置 ICE candidate 回调
func (p *Peer) OnICECandidate(callback func(candidate webrtc.ICECandidateInit)) {
	p.onICECandidate = callback
}

// OnError 设置错误回调
func (p *Peer) OnError(callback func(err error)) {
	p.onError = callback
}

// CreateOffer 创建 SDP Offer
func (p *Peer) CreateOffer() (webrtc.SessionDescription, error) {
	offer, err := p.Connection.CreateOffer(nil)
	if err != nil {
		return offer, err
	}

	if err := p.Connection.SetLocalDescription(offer); err != nil {
		return offer, err
	}

	return offer, nil
}

// CreateAnswer 创建 SDP Answer
func (p *Peer) CreateAnswer() (webrtc.SessionDescription, error) {
	answer, err := p.Connection.CreateAnswer(nil)
	if err != nil {
		return answer, err
	}

	if err := p.Connection.SetLocalDescription(answer); err != nil {
		return answer, err
	}

	return answer, nil
}

// SetRemoteSDP 设置远程 SDP
func (p *Peer) SetRemoteSDP(sdp webrtc.SessionDescription) error {
	return p.Connection.SetRemoteDescription(sdp)
}

// AddICECandidate 添加 ICE Candidate
func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return p.Connection.AddICECandidate(candidate)
}

// AddAudioTrack 添加音频轨道
func (p *Peer) AddAudioTrack() (*webrtc.TrackLocalStaticRTP, error) {
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		p.ID,
	)
	if err != nil {
		return nil, err
	}

	_, err = p.Connection.AddTrack(audioTrack)
	if err != nil {
		return nil, err
	}

	return audioTrack, nil
}

// Close 关闭连接
func (p *Peer) Close() error {
	if p.Connection == nil {
		return nil
	}
	return p.Connection.Close()
}

// IsConnected 检查是否已连接
func (p *Peer) IsConnected() bool {
	return p.Connection.ConnectionState() == webrtc.PeerConnectionStateConnected
}

// GetStats 获取连接统计
func (p *Peer) GetStats() webrtc.StatsReport {
	return p.Connection.GetStats()
}