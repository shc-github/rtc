// WebRTC Client SDK for browser
class WebRTCClient {
    constructor(serverUrl) {
        this.serverUrl = serverUrl;
        this.ws = null;
        this.peers = new Map(); // userID -> { peerConnection, audioTrack }
        this.localStream = null;
        this.localAudioTrack = null;
        this.roomId = null;
        this.userId = null;
        this.nickname = null;
        this.connected = false;
        this.sfuMode = false;
        this.sfuWS = null;
        this.sfuPC = null;
        this.isMuted = false;

        // Callbacks
        this.onUserJoined = null;
        this.onUserLeft = null;
        this.onConnectionStateChange = null;
        this.onError = null;
        this.onReady = null;
        this.onSFUSwitch = null;
        this.onMuteStateChange = null;
    }

    // Connect to signaling server
    connect() {
        return new Promise((resolve, reject) => {
            this.ws = new WebSocket(this.serverUrl);

            this.ws.onopen = () => {
                this.connected = true;
                log('WebSocket connected', 'success');
                resolve();
            };

            this.ws.onerror = (err) => {
                log('WebSocket error', 'error');
                reject(err);
            };

            this.ws.onclose = () => {
                this.connected = false;
                log('WebSocket disconnected', 'info');
                this.cleanup();
            };

            this.ws.onmessage = (event) => {
                this.handleMessage(JSON.parse(event.data));
            };
        });
    }

    // Handle signaling message
    handleMessage(msg) {
        log(`Received: ${msg.type}`, 'info');

        switch (msg.type) {
            case 'user_joined':
                if (!this.userId) {
                    // First message assigns our user ID
                    this.userId = msg.user_id;
                    log(`My user ID: ${this.userId}`, 'success');
                    if (this.onReady) this.onReady(this.userId);
                } else if (msg.user_id !== this.userId && !this.sfuMode) {
                    // Another user joined (P2P mode only)
                    log(`User joined: ${msg.user_id}`, 'info');
                    this.createPeerConnection(msg.user_id, true); // create offer
                    if (this.onUserJoined) this.onUserJoined(msg.user_id, msg.payload);
                }
                break;

            case 'user_left':
                log(`User left: ${msg.user_id}`, 'info');
                this.removePeer(msg.user_id);
                if (this.onUserLeft) this.onUserLeft(msg.user_id);
                break;

            case 'room_info':
                // Room info with existing users
                if (msg.payload && msg.payload.users) {
                    msg.payload.users.forEach(uid => {
                        if (uid !== this.userId && !this.sfuMode) {
                            log(`Existing user: ${uid}`, 'info');
                            this.createPeerConnection(uid, false); // wait for offer
                        }
                    });
                }
                // 检查是否已经是 SFU 模式
                if (msg.payload && msg.payload.sfu_mode && msg.payload.sfu_url) {
                    this.switchToSFU(msg.payload.sfu_url, msg.payload.room_id);
                }
                break;

            case 'switch_to_sfu':
                // 切换到 SFU 模式
                if (msg.payload && msg.payload.sfu_url) {
                    log('Switching to SFU mode...', 'info');
                    this.switchToSFU(msg.payload.sfu_url, msg.payload.room_id || this.roomId);
                }
                break;

            case 'offer':
                if (!this.sfuMode) {
                    this.handleOffer(msg.user_id, msg.payload);
                }
                break;

            case 'answer':
                if (!this.sfuMode) {
                    this.handleAnswer(msg.user_id, msg.payload);
                }
                break;

            case 'candidate':
                if (!this.sfuMode) {
                    this.handleCandidate(msg.user_id, msg.payload);
                }
                break;

            case 'error':
                log(`Error: ${msg.payload?.message || 'Unknown error'}`, 'error');
                if (this.onError) this.onError(msg.payload);
                break;
        }
    }

    // Join room
    joinRoom(roomId, nickname = '') {
        this.roomId = roomId;
        this.nickname = nickname;
        this.sendMessage({
            type: 'join',
            room_id: roomId,
            user_id: this.userId,
            payload: { user_id: this.userId, nickname }
        });
        log(`Joined room: ${roomId}`, 'success');
    }

    // Leave room
    leaveRoom() {
        this.sendMessage({
            type: 'leave',
            room_id: this.roomId,
            user_id: this.userId
        });
        this.cleanup();
        log('Left room', 'info');
    }

    // Switch to SFU mode
    async switchToSFU(sfuUrl, roomId) {
        log(`Switching to SFU: ${sfuUrl}`, 'info');
        this.sfuMode = true;

        // 清理所有 P2P 连接
        this.peers.forEach((peer, userId) => {
            peer.pc.close();
            if (peer.audioElement) {
                peer.audioElement.remove();
            }
        });
        this.peers.clear();

        if (this.onSFUSwitch) {
            this.onSFUSwitch(sfuUrl, roomId);
        }

        // 连接到 SFU 服务器
        try {
            const sfuWSUrl = `${sfuUrl}?user_id=${this.userId}&room_id=${roomId}`;
            this.sfuWS = new WebSocket(sfuWSUrl);

            this.sfuWS.onopen = async () => {
                log('SFU WebSocket connected', 'success');

                // 创建 SFU PeerConnection
                await this.createSFUPeerConnection();
            };

            this.sfuWS.onmessage = (event) => {
                this.handleSFUMessage(JSON.parse(event.data));
            };

            this.sfuWS.onerror = (err) => {
                log('SFU WebSocket error', 'error');
            };

            this.sfuWS.onclose = () => {
                log('SFU WebSocket closed', 'info');
                this.sfuMode = false;
            };

        } catch (err) {
            log(`SFU connection error: ${err}`, 'error');
            this.sfuMode = false;
        }
    }

    // Create SFU PeerConnection
    async createSFUPeerConnection() {
        const config = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };

        this.sfuPC = new RTCPeerConnection(config);

        // 添加本地音频轨道
        if (this.localAudioTrack) {
            log('Adding local audio track to SFU', 'info');
            this.sfuPC.addTrack(this.localAudioTrack);
        }

        // 处理接收到的轨道
        this.sfuPC.ontrack = (event) => {
            log(`Received track from SFU: ${event.track.kind}`, 'success');
            this.playSFUAudio(event.track);
        };

        // 处理 ICE candidate
        this.sfuPC.onicecandidate = (event) => {
            if (event.candidate && this.sfuWS) {
                this.sfuWS.send(JSON.stringify({
                    type: 'candidate',
                    payload: {
                        candidate: event.candidate.candidate,
                        sdp_mid: event.candidate.sdpMid,
                        sdp_m_line_index: event.candidate.sdpMLineIndex
                    }
                }));
            }
        };

        // 处理连接状态
        this.sfuPC.onconnectionstatechange = () => {
            log(`SFU connection state: ${this.sfuPC.connectionState}`, 'info');
        };

        // 创建并发送 Offer
        const offer = await this.sfuPC.createOffer({
            offerToReceiveAudio: true,
            offerToReceiveVideo: false
        });
        await this.sfuPC.setLocalDescription(offer);

        if (this.sfuWS) {
            this.sfuWS.send(JSON.stringify({
                type: 'offer',
                payload: {
                    sdp: offer.sdp,
                    type: offer.type
                }
            }));
            log('Sent offer to SFU', 'info');
        }
    }

    // Handle SFU message
    handleSFUMessage(msg) {
        log(`SFU message: ${msg.type}`, 'info');

        switch (msg.type) {
            case 'answer':
                if (this.sfuPC && msg.payload) {
                    this.sfuPC.setRemoteDescription(new RTCSessionDescription({
                        type: 'answer',
                        sdp: msg.payload.sdp
                    }));
                    log('SFU answer set', 'success');
                }
                break;

            case 'candidate':
                if (this.sfuPC && msg.payload) {
                    this.sfuPC.addIceCandidate(new RTCIceCandidate({
                        candidate: msg.payload.candidate,
                        sdpMid: msg.payload.sdp_mid,
                        sdpMLineIndex: msg.payload.sdp_m_line_index
                    }));
                    log('SFU ICE candidate added', 'info');
                }
                break;

            case 'peers':
                // 其他用户列表
                if (msg.payload && msg.payload.peers) {
                    log(`SFU peers: ${msg.payload.peers.join(', ')}`, 'info');
                    updateUserList([...msg.payload.peers, this.userId]);
                }
                break;

            case 'error':
                log(`SFU error: ${msg.payload?.message}`, 'error');
                break;
        }
    }

    // Play SFU audio
    playSFUAudio(track) {
        // SFU 会混合所有音频，所以只需要一个 audio 元素
        const audioElement = document.createElement('audio');
        audioElement.srcObject = new MediaStream([track]);
        audioElement.autoplay = true;
        audioElement.controls = false;
        audioElement.volume = 1.0;
        audioElement.id = 'audio-sfu';
        document.body.appendChild(audioElement);

        audioElement.play().catch(err => {
            log(`SFU audio play blocked: ${err}`, 'error');
        });

        log('Playing SFU audio', 'success');
    }

    // Create peer connection
    async createPeerConnection(userId, createOffer) {
        if (this.peers.has(userId)) {
            return;
        }

        log(`Creating peer connection for ${userId}`, 'info');

        const config = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };

        const pc = new RTCPeerConnection(config);

        // Add local audio track BEFORE creating offer
        if (this.localAudioTrack) {
            log(`Adding local audio track to peer ${userId}`, 'info');
            pc.addTrack(this.localAudioTrack);
        } else {
            log(`No local audio track available for peer ${userId}`, 'error');
        }

        // Handle incoming tracks
        pc.ontrack = (event) => {
            log(`Received track from ${userId}: ${event.track.kind}`, 'success');
            this.playRemoteAudio(userId, event.track);
        };

        // Handle ICE candidates
        pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.sendMessage({
                    type: 'candidate',
                    room_id: this.roomId,
                    user_id: this.userId,
                    target_id: userId,
                    payload: {
                        candidate: event.candidate.candidate,
                        sdp_mid: event.candidate.sdpMid,
                        sdp_m_line_index: event.candidate.sdpMLineIndex
                    }
                });
            }
        };

        // Handle connection state
        pc.onconnectionstatechange = () => {
            log(`Connection state with ${userId}: ${pc.connectionState}`, 'info');
            if (this.onConnectionStateChange) {
                this.onConnectionStateChange(userId, pc.connectionState);
            }
        };

        this.peers.set(userId, { pc, audioElement: null });

        if (createOffer) {
            await this.createAndSendOffer(userId);
        }
    }

    // Create and send offer
    async createAndSendOffer(userId) {
        const peer = this.peers.get(userId);
        if (!peer) return;

        try {
            const offer = await peer.pc.createOffer({
                offerToReceiveAudio: true,
                offerToReceiveVideo: false
            });
            await peer.pc.setLocalDescription(offer);

            // 检查 SDP 是否包含音频
            if (offer.sdp.includes('m=audio') || offer.sdp.includes('audio')) {
                log(`Offer contains audio for ${userId}`, 'success');
            } else {
                log(`Offer missing audio for ${userId}`, 'error');
            }

            this.sendMessage({
                type: 'offer',
                room_id: this.roomId,
                user_id: this.userId,
                target_id: userId,
                payload: {
                    sdp: offer.sdp,
                    type: offer.type
                }
            });
            log(`Sent offer to ${userId}`, 'info');
        } catch (err) {
            log(`Create offer error: ${err}`, 'error');
        }
    }

    // Handle offer from remote
    async handleOffer(userId, payload) {
        log(`Received offer from ${userId}`, 'info');

        let peer = this.peers.get(userId);
        if (!peer) {
            await this.createPeerConnection(userId, false);
            peer = this.peers.get(userId);
        }

        // 确保本地音轨已添加
        if (this.localAudioTrack && peer.pc) {
            const senders = peer.pc.getSenders();
            const hasAudioSender = senders.some(s => s.track && s.track.kind === 'audio');
            if (!hasAudioSender) {
                log(`Adding local audio track before creating answer for ${userId}`, 'info');
                peer.pc.addTrack(this.localAudioTrack);
            }
        }

        try {
            await peer.pc.setRemoteDescription(new RTCSessionDescription({
                type: 'offer',
                sdp: payload.sdp
            }));

            // 检查收到的 offer 是否包含音频
            if (payload.sdp.includes('m=audio')) {
                log(`Received offer contains audio from ${userId}`, 'success');
            } else {
                log(`Received offer missing audio from ${userId}`, 'error');
            }

            const answer = await peer.pc.createAnswer();
            await peer.pc.setLocalDescription(answer);

            // 检查 answer 是否包含音频
            if (answer.sdp.includes('m=audio') || answer.sdp.includes('audio')) {
                log(`Answer contains audio for ${userId}`, 'success');
            } else {
                log(`Answer missing audio for ${userId}`, 'error');
            }

            this.sendMessage({
                type: 'answer',
                room_id: this.roomId,
                user_id: this.userId,
                target_id: userId,
                payload: {
                    sdp: answer.sdp,
                    type: answer.type
                }
            });
            log(`Sent answer to ${userId}`, 'success');
        } catch (err) {
            log(`Handle offer error: ${err}`, 'error');
        }
    }

    // Handle answer from remote
    async handleAnswer(userId, payload) {
        log(`Received answer from ${userId}`, 'info');

        const peer = this.peers.get(userId);
        if (!peer) return;

        try {
            await peer.pc.setRemoteDescription(new RTCSessionDescription({
                type: 'answer',
                sdp: payload.sdp
            }));
            log(`Set remote description for ${userId}`, 'success');
        } catch (err) {
            log(`Handle answer error: ${err}`, 'error');
        }
    }

    // Handle ICE candidate
    async handleCandidate(userId, payload) {
        const peer = this.peers.get(userId);
        if (!peer) return;

        try {
            await peer.pc.addIceCandidate(new RTCIceCandidate({
                candidate: payload.candidate,
                sdpMid: payload.sdp_mid,
                sdpMLineIndex: payload.sdp_m_line_index
            }));
            log(`Added ICE candidate from ${userId}`, 'info');
        } catch (err) {
            log(`Add ICE candidate error: ${err}`, 'error');
        }
    }

    // Play remote audio
    playRemoteAudio(userId, track) {
        const peer = this.peers.get(userId);
        if (!peer) return;

        // 移除旧的 audio 元素
        if (peer.audioElement) {
            peer.audioElement.remove();
        }

        const audioElement = document.createElement('audio');
        audioElement.srcObject = new MediaStream([track]);
        audioElement.autoplay = true;
        audioElement.controls = false;
        audioElement.volume = 1.0;
        audioElement.id = `audio-${userId}`;
        document.body.appendChild(audioElement);

        // 尝试播放，处理 autoplay 被阻止的情况
        audioElement.play().catch(err => {
            log(`Audio play blocked for ${userId}: ${err}`, 'error');
            // 需要用户点击才能播放
        });

        peer.audioElement = audioElement;
        log(`Playing audio from ${userId}`, 'success');
    }

    // Remove peer connection
    removePeer(userId) {
        const peer = this.peers.get(userId);
        if (peer) {
            peer.pc.close();
            if (peer.audioElement) {
                peer.audioElement.remove();
            }
            this.peers.delete(userId);
        }
    }

    // Start local audio
    async startLocalAudio() {
        try {
            this.localStream = await navigator.mediaDevices.getUserMedia({
                audio: {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true
                },
                video: false
            });
            this.localAudioTrack = this.localStream.getAudioTracks()[0];

            if (this.localAudioTrack) {
                log(`Local audio track obtained: ${this.localAudioTrack.label}`, 'success');
                log(`Track settings: ${JSON.stringify(this.localAudioTrack.getSettings())}`, 'info');
            }

            // Add track to existing peers and renegotiate
            if (this.peers.size > 0) {
                this.peers.forEach((peer, userId) => {
                    if (this.localAudioTrack && peer.pc) {
                        peer.pc.addTrack(this.localAudioTrack);
                        log(`Added local audio to existing peer ${userId}`, 'info');
                        // Renegotiate
                        this.createAndSendOffer(userId);
                    }
                });
            }

            return true;
        } catch (err) {
            log(`Start audio error: ${err}`, 'error');
            return false;
        }
    }

    // Stop local audio
    stopLocalAudio() {
        if (this.localAudioTrack) {
            this.localAudioTrack.stop();
            this.localAudioTrack = null;
        }
        if (this.localStream) {
            this.localStream = null;
        }
        log('Local audio stopped', 'info');
    }

    // Toggle mute
    toggleMute() {
        if (!this.localAudioTrack) {
            log('No audio track to mute', 'error');
            return false;
        }

        this.isMuted = !this.isMuted;
        this.localAudioTrack.enabled = !this.isMuted;

        log(this.isMuted ? 'Microphone muted' : 'Microphone unmuted', 'info');

        if (this.onMuteStateChange) {
            this.onMuteStateChange(this.isMuted);
        }

        return this.isMuted;
    }

    // Set mute state
    setMute(muted) {
        if (!this.localAudioTrack) {
            return;
        }

        this.isMuted = muted;
        this.localAudioTrack.enabled = !this.isMuted;

        log(this.isMuted ? 'Microphone muted' : 'Microphone unmuted', 'info');

        if (this.onMuteStateChange) {
            this.onMuteStateChange(this.isMuted);
        }
    }

    // Get mute state
    getMuteState() {
        return this.isMuted;
    }

    // Send message
    sendMessage(msg) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            msg.timestamp = Date.now();
            this.ws.send(JSON.stringify(msg));
        }
    }

    // Cleanup
    cleanup() {
        // 清理 P2P 连接
        this.peers.forEach((peer, userId) => {
            this.removePeer(userId);
        });
        this.stopLocalAudio();
        this.roomId = null;

        // 清理 SFU 连接
        if (this.sfuPC) {
            this.sfuPC.close();
            this.sfuPC = null;
        }
        if (this.sfuWS) {
            this.sfuWS.close();
            this.sfuWS = null;
        }
        this.sfuMode = false;

        // 移除 SFU audio 元素
        const sfuAudio = document.getElementById('audio-sfu');
        if (sfuAudio) {
            sfuAudio.remove();
        }
    }

    // Disconnect
    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.cleanup();
    }
}

// Global client instance
let client = null;

// 自动检测并设置服务器地址
function getDefaultServerUrl() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    return `${protocol}//${host}/ws`;
}

// 页面加载时自动设置服务器地址
document.addEventListener('DOMContentLoaded', () => {
    const serverUrlInput = document.getElementById('server-url');
    if (serverUrlInput && !serverUrlInput.value) {
        serverUrlInput.value = getDefaultServerUrl();
    }
});

// Helper functions
function log(message, type = 'info') {
    const logPanel = document.getElementById('log-panel');
    const time = new Date().toLocaleTimeString();
    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    entry.innerHTML = `<span class="time">[${time}]</span> ${message}`;
    logPanel.appendChild(entry);
    logPanel.scrollTop = logPanel.scrollHeight;
}

function updateStatus(status) {
    const statusEl = document.getElementById('connection-status');
    statusEl.className = `status status-${status}`;
    statusEl.textContent = status === 'connected' ? '已连接' :
                           status === 'connecting' ? '连接中...' : '未连接';

    // Update audio indicator
    const audioBars = document.getElementById('audio-bars');
    audioBars.classList.toggle('silent', status !== 'connected');
}

function updateUserList(users) {
    const userList = document.getElementById('user-list');
    const userCount = document.getElementById('user-count');
    userList.innerHTML = '';
    userCount.textContent = users.length;

    users.forEach(userId => {
        const item = document.createElement('div');
        item.className = 'user-item';
        item.innerHTML = `
            <div class="user-avatar">${userId.substring(0, 2)}</div>
            <div class="user-name">${userId.substring(0, 8)}...</div>
        `;
        userList.appendChild(item);
    });
}

function updateModeIndicator(mode) {
    const modeEl = document.getElementById('mode-indicator');
    if (modeEl) {
        modeEl.textContent = mode === 'SFU' ? 'SFU 模式' : 'P2P 模式';
        modeEl.className = mode === 'SFU' ? 'mode-sfu' : 'mode-p2p';
    }
}

// Main functions
async function joinRoom() {
    const roomId = document.getElementById('room-id').value.trim();
    const nickname = document.getElementById('nickname').value.trim();
    let serverUrl = document.getElementById('server-url').value.trim();

    // 如果没有输入服务器地址，自动检测
    if (!serverUrl) {
        serverUrl = getDefaultServerUrl();
    }

    if (!roomId) {
        log('Please enter room ID', 'error');
        return;
    }

    try {
        updateStatus('connecting');
        log('Connecting to server...', 'info');

        client = new WebRTCClient(serverUrl);

        // 先获取本地音频，再连接
        log('Requesting microphone...', 'info');
        await client.startLocalAudio();

        client.onReady = (userId) => {
            document.getElementById('my-user-id').textContent = `我的 ID: ${userId}`;
            document.getElementById('my-user-id').classList.remove('hidden');
            client.joinRoom(roomId, nickname);
        };

        client.onUserJoined = (userId) => {
            updateUserList([...client.peers.keys(), client.userId]);
        };

        client.onUserLeft = (userId) => {
            updateUserList([...client.peers.keys(), client.userId]);
        };

        client.onConnectionStateChange = (userId, state) => {
            let hasConnected = false;
            client.peers.forEach((peer) => {
                if (peer.pc && peer.pc.connectionState === 'connected') {
                    hasConnected = true;
                }
            });
            // 也检查 SFU 连接状态
            if (client.sfuPC && client.sfuPC.connectionState === 'connected') {
                hasConnected = true;
            }
            updateStatus(hasConnected ? 'connected' : 'connecting');
        };

        client.onSFUSwitch = (sfuUrl, roomId) => {
            log(`Switched to SFU mode for room ${roomId}`, 'success');
            updateModeIndicator('SFU');
        };

        await client.connect();

        // Show call panel
        document.getElementById('setup-panel').classList.add('hidden');
        document.getElementById('call-panel').classList.remove('hidden');
        document.getElementById('users-panel').classList.remove('hidden');

    } catch (err) {
        log(`Connection failed: ${err}`, 'error');
        updateStatus('disconnected');
    }
}

function leaveRoom() {
    if (client) {
        client.leaveRoom();
        client.disconnect();
        client = null;
    }

    updateStatus('disconnected');
    updateUserList([]);

    // 重置静音按钮
    updateMuteButton(false);

    // Show setup panel
    document.getElementById('setup-panel').classList.remove('hidden');
    document.getElementById('call-panel').classList.add('hidden');
    document.getElementById('users-panel').classList.add('hidden');
    document.getElementById('my-user-id').classList.add('hidden');
}

function toggleMute() {
    if (!client) {
        return;
    }

    const isMuted = client.toggleMute();
    updateMuteButton(isMuted);
}

function updateMuteButton(isMuted) {
    const muteBtn = document.getElementById('mute-btn');
    if (!muteBtn) return;

    if (isMuted) {
        muteBtn.textContent = '🔇 取消静音';
        muteBtn.className = 'btn btn-muted';
    } else {
        muteBtn.textContent = '🎤 静音';
        muteBtn.className = 'btn btn-mute';
    }

    // 更新音频指示器
    const audioBars = document.getElementById('audio-bars');
    if (audioBars) {
        audioBars.classList.toggle('muted', isMuted);
    }
}