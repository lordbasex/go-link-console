// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/internal/telemetry"
	"github.com/lordbasex/go-link/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link/backend-device/pkg/input"
	"github.com/lordbasex/go-link/backend-device/pkg/inputhud"
	"github.com/lordbasex/go-link/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link/backend-device/pkg/testpattern"
	"github.com/lordbasex/go-link/backend-device/pkg/watermark"
)

// RTCSignal is the application payload carried inside signalhub "signal"
// messages to negotiate WebRTC. signalhub never reads it.
type RTCSignal struct {
	Kind      string                   `json:"kind"` // "offer", "answer" or "candidate"
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

// StreamConfig configures StreamService. Zero values get defaults.
type StreamConfig struct {
	Width, Height int // default 640x480 (4:3, like the test card)
	FPS           int // default 60, like arcade hardware
	BitrateKbps   int // default 2500 (SaverKbps); SetBitrate changes it
	// IncludeLoopback also gathers 127.0.0.1 candidates (tests on
	// machines without a network).
	IncludeLoopback bool
	// UDPPort, when set, carries every WebRTC connection on this single
	// UDP port (ICE UDP mux), so one port forward on a router is enough.
	UDPPort int
	// AnnounceIPs are extra addresses where browsers can reach UDPPort:
	// the WAN side of a router that forwards the port (for example the
	// 192.168.1.x of a second router at home, or the public IP). Each one
	// is sent as an additional host candidate.
	AnnounceIPs []string
	// API, when set, is a WebRTC API shared with other streams (the
	// device's rooms), so every room uses the same UDP port. The stream
	// then ignores IncludeLoopback; UDPPort must still be the shared port,
	// because the announced addresses point at it.
	API    *webrtc.API
	Logger *slog.Logger
	// EncoderThreads is libvpx's thread count (0: its streaming default of 2); HD sizes need more (T-31).
	EncoderThreads int
	// H264Encoder, when set ("x264" or "videotoolbox"), sends H.264 made by
	// ffmpeg instead of VP8 (go-link HD, docs/experiments/hd-streaming.md):
	// the HD test room only, without recordings or the 2x encoder check.
	H264Encoder string
	// Tiers sends each viewer the picture size it needs: the source and its
	// halves, chosen from the viewer's video_want (video_tiers.go).
	Tiers bool
}

// h264Fmtp is the H.264 the device offers: Constrained Baseline, which
// every browser decodes, sent as single NAL units and FU-A fragments.
const h264Fmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

// NewWebRTCAPI builds the WebRTC API streams share. udpPort > 0 carries
// every connection on that single UDP port (ICE UDP mux).
func NewWebRTCAPI(udpPort int, includeLoopback bool) (*webrtc.API, error) {
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(includeLoopback)
	if udpPort > 0 {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: udpPort})
		if err != nil {
			return nil, fmt.Errorf("stream: UDP port %d: %w", udpPort, err)
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, conn))
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	}
	// Pion's default codecs and interceptors, plus the playout delay
	// (playout_delay.go).
	me := &webrtc.MediaEngine{}
	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: playoutDelayURI}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	// The microphones' level on each packet (RFC 6464): who is speaking
	// gets one of the four voice slots (voice_select.go).
	if err := me.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: audioLevelURI}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	reg := &interceptor.Registry{}
	// First, so it sits closest to the network (sim_loss.go; tests only).
	if pct := simLossFromEnv(); pct > 0 {
		reg.Add(&simLossFactory{pct: pct})
	}
	if err := webrtc.RegisterDefaultInterceptors(me, reg); err != nil {
		return nil, err
	}
	reg.Add(playoutDelays)
	return webrtc.NewAPI(webrtc.WithSettingEngine(se), webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(reg)), nil
}

// PeerKind says what a peer connection carries.
type PeerKind int

const (
	// KindViewer gets video plus the control and input channels.
	KindViewer PeerKind = iota
	// KindLink is a browser linked with the pairing code: control channel
	// only, used for device status and latency.
	KindLink
)

type viewer struct {
	id      string
	kind    PeerKind
	pc      *webrtc.PeerConnection
	control *webrtc.DataChannel
	files   *webrtc.DataChannel // linked browsers: ROM uploads and downloads
	// voice[i] carries the voice of the player at port i+1 to this viewer.
	voice [4]*webrtc.TrackLocalStaticRTP

	// The video tier this viewer gets and the size it shows the video at
	// (device pixels, video_want), and its own video track; under s.mu.
	tier         int
	wantW, wantH int
	video        *webrtc.TrackLocalStaticSample
	// drop is how many quality steps this viewer is under its size's
	// tier, moved by its connection (quality_ladder.go); under s.mu.
	drop    int
	quality qualityLadder
	// videoSSRC is this viewer's video stream, to read its receiver
	// reports among the others.
	videoSSRC atomic.Uint32

	mu        sync.Mutex
	remoteSet bool
	pending   []webrtc.ICECandidateInit
	input     input.Tracker
	hidden    bool // the browser says its tab is hidden (client_report)
	playout   playoutLadder

	tm viewerTele // telemetry counters
}

// StreamService sends the video to every viewer. There is one encoder and
// one shared track: each frame is encoded once and Pion fans the packets
// out to all peer connections. Each viewer also gets two DataChannels:
// "control" (reliable, JSON) and "input" (unordered, no retransmits).
type StreamService struct {
	cfg StreamConfig
	log *slog.Logger
	// sendErrAt is when a control message that could not be sent was last
	// logged (unix nanoseconds); see sendText.
	sendErrAt atomic.Int64
	ice       *ICEStore
	api       *webrtc.API
	track     *webrtc.TrackLocalStaticSample
	audio     *webrtc.TrackLocalStaticSample

	keyframe atomic.Bool
	// rec, while the room is recorded, gets a copy of every encoded frame
	// and voice packet (nothing is encoded twice).
	rec atomic.Pointer[Recorder]
	// mark is the go-link icon drawn on the picture while recording (only
	// the video source's goroutine draws with it).
	mark atomic.Pointer[recMark]
	// hud is how many seats' controllers are drawn on the picture (0: off),
	// the host's latency test; hudBuf is the frame it is drawn on.
	hud    atomic.Int32
	hudBuf []byte
	// tele records the room's telemetry (nil: none); ftele counts its
	// frames of the last second.
	tele     atomic.Pointer[telemetry.Recorder]
	teleHost func() telemetry.Metrics // under mu
	ftele    frameTele

	// kbps is the VP8 target bitrate (SetBitrate); scale is how many times
	// the source enlarges the game's picture (SetVideoScale).
	kbps  atomic.Int64
	scale atomic.Int32
	// probeArmed asks the encoder to measure itself on the next 2x frames
	// (ArmEncodeProbe).
	probeArmed atomic.Bool

	// Owned by the source goroutine (Run): encoders and pacing.
	vp8        *encoder.VP8
	vp8W, vp8H int
	vp8Kbps    int
	h264       *encoder.H264
	tiers      []*videoTier // with StreamConfig.Tiers
	tierLevels int          // tiers the current source size has (under mu)
	// frames and bytes the H.264 reader goroutine sent since the last count
	h264Sent, h264Bytes atomic.Int64
	opus                *encoder.Opus
	pcm                 []int16
	audioFrames         int        // sound frames encoded (AudioSamples only)
	load                encodeLoad // the encode load watch (encode_load.go)
	sent                int
	window              time.Time
	probe               *encodeProbe

	mu           sync.Mutex
	sender       Sender
	viewers      map[string]*viewer
	pads         map[string][input.MaxLocalPlayers]input.Pad
	sentFPS      float64
	sentBytes    int // encoded video in the current one-second window
	videoKbps    float64
	aspect       float64
	source       MediaSource
	stopSource   func()
	onSourceErr  func(error)
	voiceOff     bool
	voices       *voiceSelector // who each voice track carries
	pings        map[uint64]time.Time
	addrs        map[string]PeerAddr // how each browser reached the device
	pingSeq      uint64
	onLatency    func(peerID string, ms int)
	room         RoomHooks
	onLinkMsg    func(peerID string, data []byte)
	linkGate     func(peerID string) bool // nil: every linked browser is trusted
	onLinkFile   func(peerID string, isString bool, data []byte)
	onLinkClosed func(peerID string)
	onLinks      func() // a linked browser's control channel opened or closed
	// videoQuality and videoFallback describe the picture for viewers
	// (stream_stats); onSlow hears the encoder check's verdict.
	videoQuality  string
	videoFallback string
	onSlow        func(p95, frame time.Duration)
}

// OnLinksChanged registers who is told that a linked browser's control
// channel opened or that its connection was removed.
func (s *StreamService) OnLinksChanged(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLinks = fn
}

// HasLink reports whether a linked browser the link gate accepts has its
// control channel open: the host is at the device's website.
func (s *StreamService) HasLink() bool {
	s.mu.Lock()
	gate := s.linkGate
	var peers []string
	for _, v := range s.viewers {
		if v.kind == KindLink && v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
			peers = append(peers, v.id)
		}
	}
	s.mu.Unlock()
	for _, p := range peers {
		if gate == nil || gate(p) {
			return true
		}
	}
	return false
}

func (s *StreamService) linksChanged() {
	s.mu.Lock()
	fn := s.onLinks
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// OnLinkFiles receives the "files" channel of linked browsers (ROM
// uploads), and OnLinkClosed tells when a linked browser goes away.
func (s *StreamService) OnLinkFiles(onData func(peerID string, isString bool, data []byte), onClosed func(peerID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLinkFile, s.onLinkClosed = onData, onClosed
}

// OnLinkMessage receives control messages from linked browsers (the
// owner), other than pongs.
func (s *StreamService) OnLinkMessage(fn func(peerID string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLinkMsg = fn
}

// RoomHooks connect room guests (KindViewer) to the Room Manager.
type RoomHooks struct {
	// Opened runs when a guest's control channel opens.
	Opened func(peerID string)
	// Message runs for every control message that is not a pong.
	Message func(peerID string, data []byte)
	// Closed runs when a guest's connection is removed.
	Closed func(peerID string)
	// PortOf maps a guest's local player to its seat (1-4). Input from
	// players without a seat is ignored.
	PortOf func(peerID string, local uint8) (int, bool)
	// VoiceRole says whether a guest may talk (a player or someone in the
	// queue) and whether it hears the voices (those, plus the spectators
	// when the room lets them). Nil: only seated players talk and hear.
	VoiceRole func(peerID string) (talk, hear bool)
	// VoiceSlots, when set, is told who each of the four voice tracks
	// carries now ("" for none), whenever that changes.
	VoiceSlots func(peers [4]string)
}

// SetRoomHooks wires the Room Manager.
func (s *StreamService) SetRoomHooks(h RoomHooks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.room = h
}

func (s *StreamService) hooks() RoomHooks {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.room
}

// announcedCandidates copies a host candidate for each announced address:
// same port and transport, another address and foundation, and a slightly
// lower priority than the real host candidate.
func announcedCandidates(host webrtc.ICECandidateInit, ips []string) []webrtc.ICECandidateInit {
	fields := strings.Fields(host.Candidate)
	// candidate:<foundation> <component> <transport> <priority> <address> <port> typ host ...
	if len(fields) < 8 || !strings.HasPrefix(fields[0], "candidate:") || fields[6] != "typ" {
		return nil
	}
	prio, err := strconv.ParseUint(fields[3], 10, 32)
	if err != nil {
		return nil
	}
	var out []webrtc.ICECandidateInit
	for i, ip := range ips {
		if net.ParseIP(ip) == nil || ip == fields[4] {
			continue
		}
		f := slices.Clone(fields)
		f[0] = fmt.Sprintf("candidate:%d", 4000000000+i)
		f[3] = strconv.FormatUint(prio-uint64(i+1)*256, 10)
		f[4] = ip
		c := host
		c.Candidate = strings.Join(f, " ")
		out = append(out, c)
	}
	return out
}

// PeerAddr is how a browser reached the device: its IP address (empty
// when it came through its own relay, which hides it) and the path,
// "direct" or "relay".
type PeerAddr struct {
	IP   string
	Path string
}

// maxPendingCandidates bounds the ICE candidates kept while an answer is
// on its way: a browser sends a handful, never hundreds.
const maxPendingCandidates = 50

// maxPeerAddrs bounds the addresses a room remembers for its history.
const maxPeerAddrs = 256

// PeerAddr returns how a browser reached the device, once it connected.
// It is kept after the browser leaves, for the history of games.
func (s *StreamService) PeerAddr(peerID string) (PeerAddr, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.addrs[peerID]
	return a, ok
}

// logPath records how a browser reached the device: directly (host,
// srflx, prflx) or through the TURN relay, which adds the round trip to
// the relay server.
func (s *StreamService) logPath(peerID string, pc *webrtc.PeerConnection) {
	sctp := pc.SCTP()
	if sctp == nil {
		return
	}
	pair, err := sctp.Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil {
		return
	}
	path := "direct"
	if pair.Local.Typ == webrtc.ICECandidateTypeRelay || pair.Remote.Typ == webrtc.ICECandidateTypeRelay {
		path = "relay"
	}
	addr := PeerAddr{Path: path}
	// Through the browser's own relay the address is the TURN server's,
	// not the player's.
	if pair.Remote.Typ != webrtc.ICECandidateTypeRelay && net.ParseIP(pair.Remote.Address) != nil {
		addr.IP = pair.Remote.Address
	}
	s.mu.Lock()
	if len(s.addrs) < maxPeerAddrs {
		s.addrs[peerID] = addr
	}
	s.mu.Unlock()
	s.mu.Lock()
	viewer := s.viewers[peerID] != nil && s.viewers[peerID].kind == KindViewer
	s.mu.Unlock()
	if viewer {
		s.teleEvent(telemetry.Info, "peer_path", peerID, "connected "+path, map[string]any{
			"path": path, "local": pair.Local.Typ.String(), "remote": pair.Remote.Typ.String(), "remote_addr": addr.IP})
	}
	s.log.Info("viewer path", "peer_id", peerID, "path", path,
		"local", fmt.Sprintf("%s %s:%d", pair.Local.Typ, pair.Local.Address, pair.Local.Port),
		"remote", fmt.Sprintf("%s %s:%d", pair.Remote.Typ, pair.Remote.Address, pair.Remote.Port))
}

// SetLinkGate filters linked browsers: status and files only reach or come
// from the ones gate accepts (a remembered browser proves its token
// first).
func (s *StreamService) SetLinkGate(gate func(peerID string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.linkGate = gate
}

func (s *StreamService) linkAllowed(peerID string) bool {
	s.mu.Lock()
	gate := s.linkGate
	s.mu.Unlock()
	return gate == nil || gate(peerID)
}

// SendControl sends a JSON message on one peer's control channel.
func (s *StreamService) SendControl(peerID string, msg []byte) bool {
	s.mu.Lock()
	v := s.viewers[peerID]
	s.mu.Unlock()
	if v == nil || v.control == nil || v.control.ReadyState() != webrtc.DataChannelStateOpen {
		return false
	}
	return s.sendText(v.control, msg)
}

// sendText sends a control message and logs, at most once a minute, the
// ones that do not go out: one larger than the browser takes (Chrome: 256
// KiB a message) is refused whole, and the browser never knows.
func (s *StreamService) sendText(dc *webrtc.DataChannel, msg []byte) bool {
	err := dc.SendText(string(msg))
	if err == nil {
		return true
	}
	now := time.Now().UnixNano()
	if last := s.sendErrAt.Load(); now-last > int64(time.Minute) && s.sendErrAt.CompareAndSwap(last, now) {
		var head struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(msg, &head)
		s.log.Warn("control message not sent", "type", head.Type, "bytes", len(msg), "err", err)
	}
	return false
}

// SendFiles sends a message on a linked browser's "files" channel
// (downloads). It reports false when the channel is not open or the
// browser has not proven its link.
func (s *StreamService) SendFiles(peerID string, isString bool, data []byte) bool {
	s.mu.Lock()
	v := s.viewers[peerID]
	s.mu.Unlock()
	if v == nil || v.kind != KindLink || v.files == nil || v.files.ReadyState() != webrtc.DataChannelStateOpen || !s.linkAllowed(peerID) {
		return false
	}
	var err error
	if isString {
		err = v.files.SendText(string(data))
	} else {
		err = v.files.Send(data)
	}
	return err == nil
}

// OnLatency registers a callback for measured round trips.
func (s *StreamService) OnLatency(fn func(peerID string, ms int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLatency = fn
}

// NewStreamService builds the service. Call Run to start encoding.
func NewStreamService(cfg StreamConfig, ice *ICEStore) (*StreamService, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		cfg.Width, cfg.Height = 640, 480
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 60
	}
	if cfg.BitrateKbps <= 0 {
		cfg.BitrateKbps = 2500
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	api := cfg.API
	if api == nil {
		var err error
		if api, err = NewWebRTCAPI(cfg.UDPPort, cfg.IncludeLoopback); err != nil {
			return nil, err
		}
	}
	video := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	if cfg.H264Encoder != "" {
		video = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264Fmtp}
	}
	track, err := newVideoTrack(video)
	if err != nil {
		return nil, err
	}
	audio, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "go-link")
	if err != nil {
		return nil, err
	}
	s := &StreamService{
		cfg:     cfg,
		log:     cfg.Logger,
		ice:     ice,
		api:     api,
		track:   track,
		audio:   audio,
		viewers: make(map[string]*viewer),
		pads:    make(map[string][input.MaxLocalPlayers]input.Pad),
		pings:   make(map[uint64]time.Time),
		addrs:   make(map[string]PeerAddr),
		voices:  newVoiceSelector(nil),
	}
	s.voices.onChange = func(peers [voiceSlots]string) {
		if fn := s.hooks().VoiceSlots; fn != nil {
			fn(peers)
		}
	}
	s.kbps.Store(int64(cfg.BitrateKbps))
	s.scale.Store(1)
	if cfg.Tiers {
		s.newTiers()
	}
	return s, nil
}

// SetBitrate changes the VP8 target bitrate; the encoder starts again
// with it on the next frame (with a keyframe).
func (s *StreamService) SetBitrate(kbps int) {
	if kbps > 0 {
		s.kbps.Store(int64(kbps))
	}
}

// Bitrate is the VP8 target bitrate in kbps.
func (s *StreamService) Bitrate() int { return int(s.kbps.Load()) }

// SetVideoScale tells how many times the source enlarges the game's
// picture (1, or 2) in the frames that follow. Viewers are told as soon
// as the encoder takes the new size, so the website averages a 2x picture
// back to the game's pixels before drawing it.
func (s *StreamService) SetVideoScale(scale int) {
	if scale != 2 {
		scale = 1
	}
	s.scale.Store(int32(scale))
}

// VideoScale is the scale of the frames being sent.
func (s *StreamService) VideoScale() int { return int(s.scale.Load()) }

// SetVideoInfo sets the video quality in use and, when lower than the
// host's choice, why (models.VideoFallbackCPU); viewers see both in
// stream_stats.
func (s *StreamService) SetVideoInfo(quality, fallback string) {
	s.mu.Lock()
	changed := s.videoQuality != quality || s.videoFallback != fallback
	s.videoQuality, s.videoFallback = quality, fallback
	s.mu.Unlock()
	if changed {
		go s.sendStreamStats()
	}
}

// ArmEncodeProbe measures the encoder over its next 2x frames (see
// encodeProbe) and calls onSlow, from its own goroutine, when 2x does not
// fit. A probe runs once per call; frames at the game's size are not
// measured.
func (s *StreamService) ArmEncodeProbe(onSlow func(p95, frame time.Duration)) {
	s.mu.Lock()
	s.onSlow = onSlow
	s.mu.Unlock()
	s.probeArmed.Store(onSlow != nil)
}

// measure feeds the encoder check with one frame's encode time.
func (s *StreamService) measure(took, dur time.Duration, w, h int) {
	if !s.probeArmed.Load() || s.VideoScale() != 2 {
		s.probe = nil
		return
	}
	if s.probe == nil {
		s.probe = newEncodeProbe(dur)
	}
	done, p95, slow := s.probe.add(took, time.Now())
	if !done {
		return
	}
	s.probe = nil
	s.probeArmed.Store(false)
	s.log.Info("video encoder check", "size", fmt.Sprintf("%dx%d", w, h), "p95_ms", float64(p95.Microseconds())/1000,
		"frame_ms", float64(dur.Microseconds())/1000, "fits", !slow)
	s.mu.Lock()
	fn := s.onSlow
	s.mu.Unlock()
	if slow && fn != nil {
		go fn(p95, dur)
	}
}

// SetSender wires the signaling client.
func (s *StreamService) SetSender(sender Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sender = sender
}

// MediaSink receives raw media from a MediaSource.
type MediaSink interface {
	// VideoFrame takes a packed I420 frame of w x h shown for dur.
	VideoFrame(i420 []byte, w, h int, dur time.Duration)
	// AudioSamples takes interleaved stereo 16-bit samples at 48 kHz.
	AudioSamples(pcm []int16)
	// SetAspect sets the display aspect ratio (e.g. 4/3) of the picture.
	SetAspect(aspect float64)
	// Controls merges the seated players' input (for the test card).
	Controls() testpattern.Controls
	// PortPad returns the controller of the player at port (1-4).
	PortPad(port int) input.Pad
}

// MediaSource produces the picture and sound: the test card, or a game.
type MediaSource interface {
	Run(ctx context.Context, sink MediaSink) error
}

// SetSource picks what Run streams. The default is the test card. While
// Run is going, the current source is stopped (a core is closed before the
// next one opens) and the new one starts.
func (s *StreamService) SetSource(src MediaSource) {
	s.mu.Lock()
	s.source = src
	stop := s.stopSource
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// OnSourceError is called when a source fails (for example, a ROM the
// core cannot load). The stream then falls back to the test card.
func (s *StreamService) OnSourceError(fn func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSourceErr = fn
}

func (s *StreamService) testCard() MediaSource {
	return &TestCardSource{Width: s.cfg.Width, Height: s.cfg.Height, FPS: s.cfg.FPS}
}

// PausableTestCard streams the test card with a pause: while paused()
// reports true the picture holds and the tone stops.
func (s *StreamService) PausableTestCard(paused func() bool) {
	card := s.testCard().(*TestCardSource)
	card.Paused = paused
	s.SetSource(card)
}

// Run streams the source until ctx ends and pings viewers every 2 s.
func (s *StreamService) Run(ctx context.Context) error {
	opus, err := encoder.NewOpus(2, 96000)
	if err != nil {
		return err
	}
	s.opus = opus
	defer func() {
		opus.Close()
		if s.vp8 != nil {
			s.vp8.Close()
		}
		if s.h264 != nil {
			s.h264.Close()
		}
		for _, t := range s.tiers {
			t.close()
		}
	}()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.pingAll()
				s.sendStreamStats()
			}
		}
	}()
	go s.teleLoop(ctx)
	for {
		s.mu.Lock()
		src := s.source
		if src == nil {
			src = s.testCard()
		}
		srcCtx, stop := context.WithCancel(ctx)
		s.stopSource = stop
		s.mu.Unlock()

		s.SetVideoScale(1) // until a source says its frames are enlarged
		s.resetFrameClock()
		err := src.Run(srcCtx, s)
		replaced := srcCtx.Err() != nil // stopped on purpose (new source or shutdown)
		stop()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && !replaced {
			// The source failed on its own: report it and go back to
			// the test card, unless someone already picked another one.
			s.log.Error("media source failed", "err", err)
			s.mu.Lock()
			cb := s.onSourceErr
			if s.source == src {
				s.source = nil
			}
			s.mu.Unlock()
			if cb != nil {
				cb(err)
			}
		}
	}
}

// VideoFrame encodes and sends one frame. The encoder follows the frame
// size, so games with odd resolutions (e.g. 248x256) are sent natively
// and the browser scales them.
func (s *StreamService) VideoFrame(i420 []byte, w, h int, dur time.Duration) {
	s.noteFrame(time.Now())
	rec := s.rec.Load()
	if s.videoViewerCount() == 0 && rec == nil {
		s.sent, s.window = 0, time.Now()
		return
	}
	if elapsed := time.Since(s.window); elapsed >= time.Second {
		s.mu.Lock()
		s.sentFPS = float64(s.sent) / elapsed.Seconds()
		s.videoKbps = float64(s.sentBytes) * 8 / 1000 / elapsed.Seconds()
		s.mu.Unlock()
		s.countLoad(elapsed, time.Now())
		s.sent, s.sentBytes, s.window = 0, 0, time.Now()
	}
	// Everything from here to the end of the frame is encoding work.
	encStart := time.Now()
	defer func() { s.load.busy += time.Since(encStart) }()
	if n := int(s.hud.Load()); n > 0 {
		// Drawn before any encoder or tier, on a copy (the source may
		// reuse its frame).
		pads := make([]input.Pad, n)
		for i := range pads {
			pads[i] = s.PortPad(i + 1)
		}
		s.hudBuf = append(s.hudBuf[:0], i420...)
		inputhud.Draw(s.hudBuf, w, h, pads)
		i420 = s.hudBuf
	}
	kbps := s.Bitrate()
	if s.tiers != nil {
		if m := s.mark.Load(); s.rec.Load() != nil && m != nil {
			i420 = m.stamp.Draw(i420, w, h, time.Since(m.start))
		}
		s.tieredFrame(i420, w, h, kbps, dur)
		return
	}
	if s.cfg.H264Encoder != "" {
		s.h264Frame(i420, w, h, kbps, dur)
		return
	}
	if s.vp8 == nil || s.vp8W != w || s.vp8H != h || s.vp8Kbps != kbps {
		if s.vp8 != nil {
			s.vp8.Close()
		}
		fps := max(int(time.Second/dur), 1)
		enc, err := encoder.NewVP8(encoder.Config{Width: w, Height: h, FPS: fps, BitrateKbps: kbps, Threads: s.cfg.EncoderThreads})
		if err != nil {
			s.log.Error("video encoder", "err", err)
			s.vp8 = nil
			return
		}
		s.mu.Lock()
		resized := s.vp8W != w || s.vp8H != h
		s.vp8, s.vp8W, s.vp8H, s.vp8Kbps = enc, w, h, kbps
		s.mu.Unlock()
		s.keyframe.Store(true)
		s.probe = nil
		if resized {
			// A new size (or scale): viewers learn it with the keyframe.
			go s.sendStreamStats()
		}
	}
	if m := s.mark.Load(); rec != nil && m != nil {
		// While recording, the icon is part of the picture: everyone sees
		// it live and the recording carries it, with no extra encoding.
		i420 = m.stamp.Draw(i420, w, h, time.Since(m.start))
	}
	start := time.Now()
	data, _, err := s.vp8.Encode(i420, s.keyframe.Swap(false))
	if err != nil {
		s.log.Error("encode failed", "err", err)
		return
	}
	took := time.Since(start)
	s.measure(took, dur, w, h)
	s.noteEncode(took, dur)
	if len(data) == 0 {
		return
	}
	if rec != nil {
		rec.Video(data, w, h)
	}
	if err := s.track.WriteSample(media.Sample{Data: data, Duration: dur}); err != nil && !errors.Is(err, errClosedPipe) {
		s.log.Debug("write sample", "err", err)
	}
	s.sent++
	s.sentBytes += len(data)
}

// h264Frame hands a frame to ffmpeg's H.264 encoder (started again when the
// size or bitrate changes); its frames are sent from the encoder's reader.
// A new viewer starts with the next keyframe, one every two seconds.
func (s *StreamService) h264Frame(i420 []byte, w, h, kbps int, dur time.Duration) {
	s.sent += int(s.h264Sent.Swap(0))
	s.sentBytes += int(s.h264Bytes.Swap(0))
	if s.h264 == nil || s.vp8W != w || s.vp8H != h || s.vp8Kbps != kbps {
		if s.h264 != nil {
			s.h264.Close()
		}
		fps := max(int(time.Second/dur), 1)
		enc, err := encoder.NewH264(encoder.Config{Width: w, Height: h, FPS: fps, BitrateKbps: kbps}, s.cfg.H264Encoder, func(au []byte) {
			if err := s.track.WriteSample(media.Sample{Data: au, Duration: dur}); err != nil && !errors.Is(err, errClosedPipe) {
				s.log.Debug("write sample", "err", err)
			}
			s.h264Sent.Add(1)
			s.h264Bytes.Add(int64(len(au)))
		})
		if err != nil {
			s.log.Error("video encoder", "codec", "h264", "err", err)
			s.h264 = nil
			return
		}
		s.log.Info("video encoder", "codec", "h264", "encoder", s.cfg.H264Encoder, "size", fmt.Sprintf("%dx%d", w, h), "kbps", kbps)
		s.mu.Lock()
		resized := s.vp8W != w || s.vp8H != h
		s.h264, s.vp8W, s.vp8H, s.vp8Kbps = enc, w, h, kbps
		s.mu.Unlock()
		if resized {
			go s.sendStreamStats()
		}
	}
	if err := s.h264.Write(i420); err != nil {
		s.log.Error("encode failed", "codec", "h264", "err", err)
		s.h264.Close()
		s.h264 = nil
	}
}

// AudioSamples buffers audio and sends it in 20 ms Opus frames.
func (s *StreamService) AudioSamples(pcm []int16) {
	rec := s.rec.Load()
	if s.videoViewerCount() == 0 && rec == nil || s.opus == nil {
		s.pcm = s.pcm[:0]
		return
	}
	s.pcm = append(s.pcm, pcm...)
	const frame = encoder.OpusFrameSamples * 2 // stereo
	for len(s.pcm) >= frame {
		// Once a second the encoder hears the worst loss among the
		// players, to make the sound sturdier where packets go missing.
		if s.audioFrames++; s.audioFrames%50 == 0 {
			s.opus.SetPacketLoss(s.worstLossPct())
		}
		pkt, err := s.opus.Encode(s.pcm[:frame])
		s.pcm = append(s.pcm[:0], s.pcm[frame:]...)
		if err != nil {
			s.log.Error("audio encode", "err", err)
			continue
		}
		if rec != nil {
			rec.GameAudio(pkt)
		}
		_ = s.audio.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond})
	}
}

// worstLossPct is the highest video loss a player's browser reported in
// its last receiver report, in percent, rounded up and kept under 30 (more
// would spend the sound's bits on redundancy for little).
func (s *StreamService) worstLossPct() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	worst := int64(0)
	for _, v := range s.viewers {
		if v.kind == KindViewer {
			worst = max(worst, v.tm.rrLossPerMi.Load())
		}
	}
	return min(int((worst+9)/10), 30)
}

// SetRecorder starts (rec) or stops (nil) copying the room's media to a
// recording. A new recording starts with a keyframe.
func (s *StreamService) SetRecorder(rec *Recorder) {
	if rec != nil {
		s.mark.Store(&recMark{stamp: watermark.New(), start: time.Now()})
	} else {
		s.mark.Store(nil)
	}
	s.rec.Store(rec)
	if rec != nil {
		s.keyframe.Store(true)
		if s.tiers != nil {
			s.tiers[0].keyframe.Store(true) // recordings take the first tier
		}
	}
}

// recMark is the icon of one recording and when it started.
type recMark struct {
	stamp *watermark.Stamper
	start time.Time
}

// SetInputHUD draws the controllers of seats 1 to seats on the picture
// (0 turns it off) and tells the viewers where each seat's beacon is.
func (s *StreamService) SetInputHUD(seats int) {
	n := int32(min(max(seats, 0), input.MaxLocalPlayers))
	if s.hud.Swap(n) != n {
		go s.sendStreamStats()
	}
}

// SetAspect records the picture's display aspect ratio for the web.
func (s *StreamService) SetAspect(aspect float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aspect = aspect
}

var errClosedPipe = errors.New("io: read/write on closed pipe")

func (s *StreamService) videoViewerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, v := range s.viewers {
		if v.kind == KindViewer {
			n++
		}
	}
	return n
}

// ViewerCount returns how many peer connections exist.
func (s *StreamService) ViewerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.viewers)
}

// Pressed returns the buttons a viewer is holding, all local players
// together.
func (s *StreamService) Pressed(peerID string) input.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all input.State
	for _, pad := range s.pads[peerID] {
		all |= pad.Buttons
	}
	return all
}

// PortPad merges the controllers of whoever is seated at port (1-4).
// Without a Room Manager, local player N of every guest plays port N+1.
// The port's Start is also down while any seated player presses its panel
// start button (Start1 to Start4).
func (s *StreamService) PortPad(port int) input.Pad {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out input.Pad
	panelStart := input.StartOf(port)
	for peer, pads := range s.pads {
		for i, pad := range pads {
			p := i + 1
			if s.room.PortOf != nil {
				var ok bool
				if p, ok = s.room.PortOf(peer, uint8(i)); !ok {
					continue
				}
			}
			// Any seated player can press this port's start button.
			if panelStart != 0 && pad.Buttons.Pressed(panelStart) {
				out.Buttons |= input.State(input.Start)
			}
			if p != port {
				continue
			}
			out.Buttons |= pad.Buttons
			for a := range pad.Axes {
				if abs(int(pad.Axes[a])) > abs(int(out.Axes[a])) {
					out.Axes[a] = pad.Axes[a]
				}
			}
		}
	}
	return out
}

// Stats returns what is being streamed. With no viewer nothing is encoded.
func (s *StreamService) Stats() models.StreamStatus {
	viewers := s.videoViewerCount()
	s.mu.Lock()
	defer s.mu.Unlock()
	if viewers == 0 {
		return models.StreamStatus{}
	}
	return models.StreamStatus{FPS: s.sentFPS, Width: s.vp8W, Height: s.vp8H, VideoKbps: s.videoKbps, VideoViewers: viewers, Scale: s.VideoScale()}
}

// SentFPS returns the frames per second sent over the last second.
func (s *StreamService) SentFPS() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sentFPS
}

// Controls merges the input of every seated player for the test card.
// The lamps show seats (ports), not local player numbers. Without a Room
// Manager every local player counts, by its own number.
func (s *StreamService) Controls() testpattern.Controls {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := testpattern.Controls{FPS: s.sentFPS}
	strongest := 0
	for peer, pads := range s.pads {
		for i, pad := range pads {
			port := i + 1
			if s.room.PortOf != nil {
				var ok bool
				if port, ok = s.room.PortOf(peer, uint8(i)); !ok {
					continue // not seated: watching or in the queue
				}
			}
			c.Buttons |= pad.Buttons
			move := 0
			for _, a := range pad.Axes {
				move += abs(int(a))
			}
			if (pad.Buttons != 0 || move != 0) && port >= 1 && port <= len(c.Players) {
				c.Players[port-1] = true
			}
			if move > strongest {
				strongest, c.Axes = move, pad.Axes
			}
		}
	}
	return c
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// sendStreamStats tells viewers what the device is sending, so the web
// can compare sent and received frames.
func (s *StreamService) sendStreamStats() {
	type target struct {
		dc  *webrtc.DataChannel
		msg []byte
	}
	s.mu.Lock()
	all, _ := json.Marshal(s.streamStatsLocked())
	var targets []target
	for _, v := range s.viewers {
		if v.kind == KindViewer && v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
			msg := all
			if s.tiers != nil {
				// each viewer hears of its own tier's picture
				st := s.streamStatsLocked()
				l := s.sizeOfLocked(s.tierOfLocked(v))
				st.Width, st.Height = s.vp8W>>l, s.vp8H>>l
				st.Video = &StreamVideo{Scale: 1, Width: st.Width, Height: st.Height}
				msg, _ = json.Marshal(st)
			}
			targets = append(targets, target{v.control, msg})
		}
	}
	s.mu.Unlock()
	for _, t := range targets {
		_ = t.dc.SendText(string(t.msg))
	}
}

// StreamVideo is the picture a room sends (stream_stats "video"): Scale is
// 2 when every game pixel is sent as a 2x2 block, and Width x Height is
// the game's own size (the frames are Scale times larger).
type StreamVideo struct {
	Scale    int    `json:"scale"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Quality  string `json:"quality,omitempty"`  // high, normal or saver (game rooms)
	Fallback string `json:"fallback,omitempty"` // "cpu": lower than the host's choice
}

// streamStats is the stream_stats message.
type streamStats struct {
	Type   string       `json:"type"`
	FPS    float64      `json:"fps"`
	Width  int          `json:"width"`  // the frames sent
	Height int          `json:"height"` // (Video has the game's size)
	Aspect float64      `json:"aspect"`
	Video  *StreamVideo `json:"video,omitempty"` // nil before the first frame
	// HUD is where each seat's beacon is while the controllers are drawn
	// on the picture (P1 first).
	HUD []inputhud.Rect `json:"hud,omitempty"`
}

func (s *StreamService) streamStatsLocked() streamStats {
	st := streamStats{Type: "stream_stats", FPS: s.sentFPS, Width: s.vp8W, Height: s.vp8H, Aspect: s.aspect}
	if scale := s.VideoScale(); s.vp8W > 0 && s.vp8H > 0 && s.vp8W%scale == 0 && s.vp8H%scale == 0 {
		st.Video = &StreamVideo{Scale: scale, Width: s.vp8W / scale, Height: s.vp8H / scale, Quality: s.videoQuality, Fallback: s.videoFallback}
	}
	if n := int(s.hud.Load()); n > 0 && s.vp8W > 0 && s.vp8H > 0 {
		st.HUD = inputhud.Beacons(s.vp8W, s.vp8H, n)
	}
	return st
}

func (s *StreamService) sendSignal(to string, sig RTCSignal) {
	s.mu.Lock()
	sender := s.sender
	s.mu.Unlock()
	if sender == nil {
		return
	}
	payload, err := json.Marshal(sig)
	if err != nil {
		return
	}
	if err := sender.Send(signalclient.Envelope{Type: signalclient.TypeSignal, To: to, Payload: payload}); err != nil {
		s.log.Warn("signal not sent", "to", to, "kind", sig.Kind, "err", err)
	}
}

func (s *StreamService) iceServers() []webrtc.ICEServer {
	var out []webrtc.ICEServer
	for _, srv := range s.ice.Get() {
		out = append(out, webrtc.ICEServer{URLs: srv.URLs, Username: srv.Username, Credential: srv.Credential})
	}
	return out
}

// AddViewer opens a video peer connection to peerID and sends an offer.
func (s *StreamService) AddViewer(peerID string) error { return s.AddPeer(peerID, KindViewer) }

// AddLink opens a data-only peer connection to a linked browser.
func (s *StreamService) AddLink(peerID string) error { return s.AddPeer(peerID, KindLink) }

// AddPeer opens a peer connection of the given kind and sends an offer.
func (s *StreamService) AddPeer(peerID string, kind PeerKind) error {
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{ICEServers: s.iceServers()})
	if err != nil {
		return err
	}
	v := &viewer{id: peerID, kind: kind, pc: pc}
	v.tm.ctlRttMs.Store(-1)
	fail := func(err error) error {
		_ = pc.Close()
		return err
	}

	if kind == KindViewer {
		if err := s.addMedia(v); err != nil {
			return fail(err)
		}
	}

	ordered := true
	control, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{Ordered: &ordered})
	if err != nil {
		return fail(err)
	}
	v.control = control
	control.OnOpen(func() {
		if kind == KindViewer {
			_ = control.SendText(`{"type":"welcome","source":"test-pattern"}`)
			s.mu.Lock()
			stats, _ := json.Marshal(s.streamStatsLocked())
			s.mu.Unlock()
			_ = control.SendText(string(stats))
			if h := s.hooks(); h.Opened != nil {
				h.Opened(peerID)
			}
		}
		if kind == KindLink {
			s.linksChanged()
		}
	})
	control.OnMessage(func(msg webrtc.DataChannelMessage) { s.handleControl(v, msg.Data) })

	if kind == KindLink {
		// ROM uploads from the owner: reliable and ordered, and separate
		// from control so a large file never delays other messages.
		files, err := pc.CreateDataChannel("files", &webrtc.DataChannelInit{Ordered: &ordered})
		if err != nil {
			return fail(err)
		}
		v.files = files
		files.OnMessage(func(msg webrtc.DataChannelMessage) {
			s.mu.Lock()
			fn := s.onLinkFile
			s.mu.Unlock()
			if fn != nil && s.linkAllowed(peerID) {
				fn(peerID, msg.IsString, msg.Data)
			}
		})
	}

	if kind == KindViewer {
		unordered := false
		noRetransmits := uint16(0)
		inputDC, err := pc.CreateDataChannel("input", &webrtc.DataChannelInit{Ordered: &unordered, MaxRetransmits: &noRetransmits})
		if err != nil {
			return fail(err)
		}
		inputDC.OnMessage(func(msg webrtc.DataChannelMessage) { s.handleInput(v, msg.Data) })
	}
	return s.offer(v)
}

// addMedia adds, in this order: game video, game audio, one voice track
// per port (P1-P4) and a receive-only audio line for the guest's
// microphone. Every track exists from the first offer, so taking or
// losing a seat never renegotiates the connection.
func (s *StreamService) addMedia(v *viewer) error {
	pc := v.pc
	track := s.track
	if s.tiers != nil {
		// a tiered stream gives every viewer its own track, one size down
		// until the viewer says what it shows (video_tiers.go)
		own, err := newVideoTrack(s.track.Codec())
		if err != nil {
			return err
		}
		s.mu.Lock()
		v.tier = min(1, max(s.tierLevels-1, 0))
		v.video = own
		s.mu.Unlock()
		track = own
	}
	rtpSender, err := pc.AddTrack(track)
	if err != nil {
		return err
	}
	if enc := rtpSender.GetParameters().Encodings; len(enc) > 0 {
		v.videoSSRC.Store(uint32(enc[0].SSRC))
	}
	// Read RTCP: a Picture Loss Indication means the viewer cannot decode
	// and needs a keyframe.
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := rtpSender.Read(buf)
			if err != nil {
				return
			}
			pkts, err := rtcp.Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			s.noteRTCP(v, pkts)
			for _, p := range pkts {
				switch p.(type) {
				case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
					if s.tiers != nil {
						s.mu.Lock()
						l := s.tierOfLocked(v)
						s.mu.Unlock()
						s.tiers[l].keyframe.Store(true)
					}
					s.keyframe.Store(true)
				}
			}
		}
	}()

	senders := []*webrtc.RTPSender{}
	audioSender, err := pc.AddTrack(s.audio)
	if err != nil {
		return err
	}
	senders = append(senders, audioSender)
	for i := range v.voice {
		id := fmt.Sprintf("voice-p%d", i+1)
		track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, id, id)
		if err != nil {
			return err
		}
		sender, err := pc.AddTrack(track)
		if err != nil {
			return err
		}
		v.voice[i] = track
		senders = append(senders, sender)
	}
	for _, sender := range senders {
		go func() { // drain RTCP so Pion's interceptors keep working
			buf := make([]byte, 1500)
			for {
				if _, _, err := sender.Read(buf); err != nil {
					return
				}
			}
		}()
	}

	// The guest's microphone.
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		return err
	}
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			level := 0
			for _, e := range receiver.GetParameters().HeaderExtensions {
				if e.URI == audioLevelURI {
					level = e.ID
				}
			}
			s.forwardVoice(v.id, track, level)
		}
	})
	return nil
}

// audioLevelURI is the RTP header extension with a microphone's level.
const audioLevelURI = "urn:ietf:params:rtp-hdrext:ssrc-audio-level"

// forwardVoice relays a guest's microphone, packet by packet and without
// decoding (SFU style), on the voice slot it speaks in, to everyone else
// who hears the voices. Audio from a guest who may not talk (a spectator)
// is dropped here, on the device, whatever the web shows. levelID is the
// negotiated id of the audio level extension (0: none, then the packet's
// size tells speech from silence).
func (s *StreamService) forwardVoice(from string, track *webrtc.TrackRemote, levelID int) {
	s.mu.Lock()
	v := s.viewers[from]
	s.mu.Unlock()
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			s.voices.Drop(from)
			return
		}
		if v != nil {
			v.tm.voiceIn.Add(1)
		}
		if talk, _ := s.voiceRole(from); !talk || s.voiceDisabled() {
			s.voices.Drop(from)
			continue // may not talk, or voice is off in this room
		}
		slot := s.voices.Pick(from, voiceIsLoud(pkt, levelID), pkt, levelID)
		if slot < 0 {
			continue // four others are speaking
		}
		if rec := s.rec.Load(); rec != nil {
			rec.Voice(slot+1, pkt)
		}
		for _, out := range s.voiceTargets(from, slot) {
			_ = out.WriteRTP(pkt)
		}
	}
}

// voiceIsLoud tells speech from silence: the level the browser put on the
// packet, or without it the packet's size (Opus spends few bytes on
// silence).
func voiceIsLoud(pkt *rtp.Packet, levelID int) bool {
	if levelID > 0 {
		if b := pkt.GetExtension(uint8(levelID)); len(b) > 0 {
			return int(b[0]&0x7f) <= voiceLoud
		}
	}
	return len(pkt.Payload) > 40
}

// voiceRole is the room's rule for one guest; without one, seated
// players talk and hear.
func (s *StreamService) voiceRole(peerID string) (talk, hear bool) {
	s.mu.Lock()
	fn := s.room.VoiceRole
	s.mu.Unlock()
	if fn != nil {
		return fn(peerID)
	}
	seated := s.seatOf(peerID) != 0
	return seated, seated
}

// VoicePeers is who each voice track carries now ("" for none).
func (s *StreamService) VoicePeers() [4]string { return s.voices.Peers() }

// SetVoiceEnabled turns voice between players on or off for the room.
func (s *StreamService) SetVoiceEnabled(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.voiceOff = !on
}

func (s *StreamService) voiceDisabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.voiceOff
}

// seatOf returns the first port held by any local player of peerID, or 0.
func (s *StreamService) seatOf(peerID string) int {
	s.mu.Lock()
	portOf := s.room.PortOf
	s.mu.Unlock()
	if portOf == nil {
		return 0
	}
	for local := uint8(0); local < input.MaxLocalPlayers; local++ {
		if port, ok := portOf(peerID, local); ok {
			return port
		}
	}
	return 0
}

// voiceTargets lists the tracks of voice slot that reach everyone allowed
// to hear the voices. The speaker never gets their own voice back.
func (s *StreamService) voiceTargets(from string, slot int) []*webrtc.TrackLocalStaticRTP {
	s.mu.Lock()
	viewers := make([]*viewer, 0, len(s.viewers))
	for _, v := range s.viewers {
		if v.kind == KindViewer && v.id != from {
			viewers = append(viewers, v)
		}
	}
	s.mu.Unlock()
	var out []*webrtc.TrackLocalStaticRTP
	for _, v := range viewers {
		if _, hear := s.voiceRole(v.id); hear && v.voice[slot] != nil {
			out = append(out, v.voice[slot])
		}
	}
	return out
}

func (s *StreamService) offer(v *viewer) error {
	pc, peerID := v.pc, v.id
	fail := func(err error) error {
		_ = pc.Close()
		return err
	}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering finished
		}
		init := c.ToJSON()
		s.sendSignal(peerID, RTCSignal{Kind: "candidate", Candidate: &init})
		// The same port behind a forwarding router: announce it there too.
		if c.Typ == webrtc.ICECandidateTypeHost && s.cfg.UDPPort > 0 && int(c.Port) == s.cfg.UDPPort {
			for _, extra := range announcedCandidates(init, s.cfg.AnnounceIPs) {
				s.sendSignal(peerID, RTCSignal{Kind: "candidate", Candidate: &extra})
			}
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.log.Info("viewer connection", "peer_id", peerID, "state", state.String())
		if v.kind == KindViewer {
			level := telemetry.Info
			if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateDisconnected {
				level = telemetry.Warn
			}
			s.teleEvent(level, "peer_state", peerID, "connection "+state.String(), nil)
		}
		switch state {
		case webrtc.PeerConnectionStateConnected:
			s.keyframe.Store(true)
			go s.logPath(peerID, pc)
		case webrtc.PeerConnectionStateFailed:
			go s.RemoveViewer(peerID) // not from inside Pion's callback
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fail(err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fail(err)
	}

	s.mu.Lock()
	old := s.viewers[peerID]
	s.viewers[peerID] = v
	s.mu.Unlock()
	if old != nil {
		_ = old.pc.Close()
	}
	if v.kind == KindViewer {
		s.keyframe.Store(true)
	}
	s.sendSignal(peerID, RTCSignal{Kind: "offer", SDP: offer.SDP})
	return nil
}

// controlMessage is the envelope of every JSON message on "control".
// controlMessage is the part of a control message the stream reads. The
// id stays raw: a pong carries a number, but other messages (room actions)
// carry a text id, and they must not be dropped for it.
type controlMessage struct {
	Type string          `json:"type"`
	ID   json.RawMessage `json:"id,omitempty"`
}

func (s *StreamService) handleControl(v *viewer, data []byte) {
	var msg controlMessage
	if json.Unmarshal(data, &msg) != nil {
		return
	}
	if msg.Type == "client_report" && v.kind == KindViewer {
		s.noteClientReport(v, data)
		return
	}
	if msg.Type == "video_want" && v.kind == KindViewer {
		var want struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if json.Unmarshal(data, &want) == nil {
			s.setVideoWant(v, want.Width, want.Height)
		}
		return
	}
	if msg.Type != "pong" {
		s.mu.Lock()
		h, linkMsg := s.room, s.onLinkMsg
		s.mu.Unlock()
		switch {
		case v.kind == KindViewer && h.Message != nil:
			h.Message(v.id, data)
		case v.kind == KindLink && linkMsg != nil:
			linkMsg(v.id, data)
		}
		return
	}
	var id uint64
	if json.Unmarshal(msg.ID, &id) != nil {
		return
	}
	s.mu.Lock()
	sent, ok := s.pings[id]
	delete(s.pings, id)
	cb := s.onLatency
	s.mu.Unlock()
	if ok {
		v.tm.ctlRttMs.Store(time.Since(sent).Milliseconds())
	}
	if ok && cb != nil {
		cb(v.id, int(time.Since(sent).Milliseconds()))
	}
}

// pingAll sends a ping to every open control channel. The browser echoes
// it, which measures the round trip over the peer-to-peer path.
func (s *StreamService) pingAll() {
	s.mu.Lock()
	now := time.Now()
	for id, t := range s.pings { // forget pings that never came back
		if now.Sub(t) > 10*time.Second {
			delete(s.pings, id)
		}
	}
	var targets []*webrtc.DataChannel
	var ids []uint64
	for _, v := range s.viewers {
		if v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen {
			s.pingSeq++
			s.pings[s.pingSeq] = now
			targets = append(targets, v.control)
			ids = append(ids, s.pingSeq)
		}
	}
	s.mu.Unlock()
	for i, dc := range targets {
		b, _ := json.Marshal(struct {
			Type string `json:"type"`
			ID   uint64 `json:"id"`
		}{"ping", ids[i]})
		_ = dc.SendText(string(b))
	}
}

// SendToLinks sends a JSON message to every linked browser. Viewers of a
// room never receive it: device details are only for the owner.
func (s *StreamService) SendToLinks(msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.mu.Lock()
	var targets []*webrtc.DataChannel
	gate := s.linkGate
	for _, v := range s.viewers {
		if v.kind == KindLink && v.control != nil && v.control.ReadyState() == webrtc.DataChannelStateOpen && (gate == nil || gate(v.id)) {
			targets = append(targets, v.control)
		}
	}
	s.mu.Unlock()
	for _, dc := range targets {
		s.sendText(dc, b)
	}
}

// HandleSignal applies an answer or ICE candidate from a viewer.
func (s *StreamService) HandleSignal(from string, payload json.RawMessage) error {
	s.mu.Lock()
	v := s.viewers[from]
	s.mu.Unlock()
	if v == nil {
		return errors.New("stream: signal from unknown viewer")
	}
	var sig RTCSignal
	if err := json.Unmarshal(payload, &sig); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	switch sig.Kind {
	case "answer":
		if err := v.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sig.SDP}); err != nil {
			return err
		}
		v.remoteSet = true
		for _, c := range v.pending {
			_ = v.pc.AddICECandidate(c)
		}
		v.pending = nil
	case "candidate":
		if sig.Candidate == nil {
			return nil
		}
		s.log.Debug("remote candidate", "peer_id", from, "candidate", sig.Candidate.Candidate)
		if !v.remoteSet {
			if len(v.pending) < maxPendingCandidates {
				v.pending = append(v.pending, *sig.Candidate) // arrived before the answer
			}
			return nil
		}
		return v.pc.AddICECandidate(*sig.Candidate)
	default:
		return errors.New("stream: unknown signal kind " + sig.Kind)
	}
	return nil
}

func (s *StreamService) handleInput(v *viewer, packet []byte) {
	v.mu.Lock()
	s.noteInput(v, packet, time.Now())
	pkt, changed, err := v.input.Apply(packet)
	pads := v.input.Pads()
	v.mu.Unlock()
	if err != nil {
		return
	}
	s.mu.Lock()
	if _, ok := s.viewers[v.id]; ok {
		s.pads[v.id] = pads
	}
	s.mu.Unlock()
	if changed {
		s.log.Debug("input", "peer_id", v.id, "player", pkt.Player, "buttons", pkt.Pad.Buttons.String())
	}
}

// RemoveViewer closes the peer connection of peerID, if any.
func (s *StreamService) RemoveViewer(peerID string) {
	s.mu.Lock()
	v := s.viewers[peerID]
	delete(s.viewers, peerID)
	delete(s.pads, peerID)
	closed := s.room.Closed
	linkClosed := s.onLinkClosed
	s.mu.Unlock()
	if v != nil {
		_ = v.pc.Close()
		if v.kind == KindViewer && closed != nil {
			closed(peerID)
		}
		if v.kind == KindLink && linkClosed != nil {
			linkClosed(peerID)
		}
		if v.kind == KindLink {
			s.linksChanged()
		}
	}
}

// CloseAll drops every viewer (the signaling link was lost).
func (s *StreamService) CloseAll() {
	s.mu.Lock()
	ids := slices.Collect(maps.Keys(s.viewers))
	s.mu.Unlock()
	for _, id := range ids {
		s.RemoveViewer(id)
	}
}
