package webrtc

import (
	"github.com/pion/webrtc/v4"
)

// GetDefaultMediaEngine 获取默认媒体引擎（支持 Opus 音频）
func GetDefaultMediaEngine() *webrtc.MediaEngine {
	m := webrtc.MediaEngine{}

	// 注册 Opus 音频编解码器
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels: 2,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio)

	return &m
}

// GetOpusCodec 获取 Opus 编解码器参数
func GetOpusCodec() webrtc.RTPCodecCapability {
	return webrtc.RTPCodecCapability{
		MimeType: webrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels: 2,
		SDPFmtpLine: "minptime=10;useinbandfec=1",
	}
}

// CreateAudioTrack 创建音频轨道
func CreateAudioTrack(peerID string) (*webrtc.TrackLocalStaticRTP, error) {
	return webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels: 2,
		},
		"audio",
		peerID,
	)
}