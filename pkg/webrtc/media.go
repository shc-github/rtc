package webrtc

import (
	"io"
	"log"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// AudioHandler 音频处理器
type AudioHandler struct {
	track    *webrtc.TrackLocalStaticRTP
	packets  chan *rtp.Packet
	stop     chan struct{}
}

// NewAudioHandler 创建音频处理器
func NewAudioHandler(track *webrtc.TrackLocalStaticRTP) *AudioHandler {
	return &AudioHandler{
		track:   track,
		packets: make(chan *rtp.Packet, 100),
		stop:    make(chan struct{}),
	}
}

// WriteRTP 写入 RTP 包
func (h *AudioHandler) WriteRTP(packet *rtp.Packet) error {
	select {
	case h.packets <- packet:
		return nil
	default:
		log.Printf("AudioHandler: packet buffer full, dropping packet")
		return nil
	}
}

// Start 开始发送
func (h *AudioHandler) Start() {
	go func() {
		for {
			select {
			case packet := <-h.packets:
				if err := h.track.WriteRTP(packet); err != nil {
					if err == io.EOF {
						return
					}
					log.Printf("AudioHandler write error: %v", err)
				}
			case <-h.stop:
				return
			}
		}
	}()
}

// Stop 停止发送
func (h *AudioHandler) Stop() {
	close(h.stop)
}

// ForwardTrack 转发远程轨道到本地轨道
func ForwardTrack(remoteTrack *webrtc.TrackRemote, localTrack *webrtc.TrackLocalStaticRTP) error {
	go func() {
		for {
			rtpPacket, _, err := remoteTrack.ReadRTP()
			if err != nil {
				if err == io.EOF {
					log.Printf("Track forward ended: %s", remoteTrack.ID())
					return
				}
				log.Printf("Track read error: %v", err)
				continue
			}

			if err := localTrack.WriteRTP(rtpPacket); err != nil {
				log.Printf("Track write error: %v", err)
			}
		}
	}()

	return nil
}

// ReadTrackPackets 从轨道读取 RTP 包
func ReadTrackPackets(track *webrtc.TrackRemote, handler func(packet *rtp.Packet)) {
	go func() {
		for {
			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				if err == io.EOF {
					log.Printf("Track reading ended: %s", track.ID())
					return
				}
				log.Printf("Track read error: %v", err)
				continue
			}

			handler(rtpPacket)
		}
	}()
}