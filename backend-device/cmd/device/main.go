// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command device runs the go-link host: it links browsers through
// signalhub and serves the local panel. The emulator arrives in a later
// phase.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/internal/panel"
	"github.com/lordbasex/go-link/backend-device/internal/services"
	"github.com/lordbasex/go-link/backend-device/internal/telemetry"
	"github.com/lordbasex/go-link/backend-device/pkg/encoder"
	"github.com/lordbasex/go-link/backend-device/pkg/input"
	"github.com/lordbasex/go-link/backend-device/pkg/instancelock"
	"github.com/lordbasex/go-link/backend-device/pkg/jsonkeys"
	"github.com/lordbasex/go-link/backend-device/pkg/libretro"
	"github.com/lordbasex/go-link/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link/backend-device/pkg/thumbnails"
	"github.com/lordbasex/go-link/backend-device/web"
)

// version is replaced at build time with -ldflags "-X main.version=...".
var version = "0.1.0-dev"

func main() {
	// Subcommands (device roms check, device core download...) manage
	// the device from a terminal, with no graphical interface.
	if handled, err := runCommand(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "device:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "device:", err)
		os.Exit(1)
	}
}

func run() error {
	dumpOnSignal()
	var (
		configPath = flag.String("config", "", "path to device.json (default: user config dir)")
		signalURL  = flag.String("server-signaling", "", "signaling server URL for this run, e.g. wss://signal.example.com/ws (default: signal_url in device.json, else "+models.DefaultSignalURL+")")
		headless   = flag.Bool("headless", false, "run without window or tray icon (servers, Raspberry Pi); the pairing code goes to the log")
		debug      = flag.Bool("debug", false, "verbose logs")
		testRoom   = flag.Bool("test-room", true, "open the test pattern room (or the game given with --game)")
		testPause  = flag.Bool("test-room-pause", false, "let the host pause the test pattern room (to try the pause and its requests without a game)")
		testHD     = flag.String("test-room-hd", "", "go-link HD's experiment (T-31): the test room streams an HD scene (720p, 1080p, 2160p, or auto: the best this computer streams at 60 fps, checked) made of --hd-far and --hd-play")
		hdCore     = flag.String("hd-core", "", "go-link HD's engine library (default: the one shipped with the device, else libgolinkhd in the cores folder): game rooms of .glhd packages run it; with --test-room-hd 720p, 1080p or 2160p the test room plays its built-in demo (its 640x360 screen enlarged x2, x3 or x6)")
		hdRoomSize = flag.String("hd-room-size", "", "go-link HD game rooms' picture for this run: 720p (x2) or 1080p (x3) (default: go-link HD's Screen setting, else 720p)")
		hdRoomCdc  = flag.String("hd-room-codec", "", "go-link HD game rooms' codec for this run: vp8, h264 (--hd-h264's encoder) or auto (the Mac's hardware H.264 through VideoToolbox, vp8 elsewhere) (default: go-link HD's Screen setting, else auto)")
		hdRoomKbps = flag.Int("hd-room-kbps", 0, "go-link HD game rooms' bitrate at high quality (default by size: 4000 at 720p, 8000 at 1080p)")
		hdFar      = flag.String("hd-far", "", "the HD scene's far picture (with --test-room-hd)")
		hdPlay     = flag.String("hd-play", "", "the HD scene's play picture, #FF00FF transparent (with --test-room-hd)")
		hdKbps     = flag.Int("hd-kbps", 0, "the HD scene's VP8 bitrate (default by size: 4000, 8000, 25000)")
		hdThreads  = flag.Int("hd-threads", 8, "libvpx threads for the HD scene")
		hdCodec    = flag.String("hd-codec", "vp8", "the HD scene's codec: vp8, or h264 made by ffmpeg (needs ffmpeg installed)")
		hdH264     = flag.String("hd-h264", "x264", "with --hd-codec h264: x264 (software) or videotoolbox (the Mac's hardware)")
		game       = flag.String("game", "", "ROM set to play in the room, e.g. robby (from the ROM folder); empty streams the test pattern")
		udpPort    = flag.Int("udp-port", 0, "carry every WebRTC connection on this UDP port, to forward it on a router (default: udp_port in device.json, else random ports)")
		announce   = flag.String("announce", "", "comma-separated addresses where browsers reach --udp-port through a forwarding router (default: announce_ips in device.json)")
		webURL     = flag.String("web-url", "", "website shown in the window for linking, for this run (default: web_url in device.json, else "+models.DefaultWebURL+")")
		corePath   = flag.String("core", "", "libretro core to run games (default: ~/go-link/cores/mame2003_plus_libretro)")
		panelAddr  = flag.String("panel", panel.DefaultAddr, "address of the local web panel, which a headless device opens on the LAN (token: `device panel token`)")
		noPanel    = flag.Bool("no-panel", false, "do not open the local web panel when running headless")
	)
	flag.Usage = usage
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logOut, closeLog := logOutput()
	defer closeLog()
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	store, cfg, created, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	// One device per user: a locked file, since the device opens no port.
	lock, err := instancelock.Acquire(filepath.Join(filepath.Dir(store.Path()), "device.lock"))
	if err != nil {
		return fmt.Errorf("cannot start: %w", err)
	}
	defer lock.Release()
	target, err := resolveSignalURL(*signalURL, cfg)
	if err != nil {
		return err
	}
	logger.Info("device starting", "version", version, "device_id", cfg.DeviceID, "config", store.Path(), "first_run", created, "signaling", target)
	if encrypted, _ := signalclient.CheckURL(target); !encrypted && !isLoopback(target) {
		logger.Warn("signaling URL is not encrypted (ws://); use wss:// outside local development", "url", target)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	status, library, settings, base, err := openLibrary(store, &cfg, target, *corePath, logger)
	if err != nil {
		return err
	}
	go library.RefreshCatalog(ctx)
	if !guiBuilt && !*headless {
		*headless = true // a build without a window
	}
	if wantsHeadless() && !*headless {
		logger.Info("no desktop found, running headless")
		*headless = true
	}
	ice := services.NewICEStore()
	pairing := services.NewPairingService(services.PairingConfig{DeviceID: cfg.DeviceID, DeviceSecret: cfg.DeviceSecret, Logger: logger}, status, ice)
	// Rooms share one signaling connection; the opener gives each its own
	// room_id (it must see room_opened before anyone else).
	opener := services.NewRoomOpener()
	handlers := services.Dispatcher{opener, pairing}

	// WebRTC peers: linked browsers get a data link (status, latency);
	// room guests get the video.
	port, ips := cfg.UDPPort, cfg.AnnounceIPs
	if *udpPort > 0 {
		port = *udpPort
	}
	if *announce != "" {
		ips = strings.Split(*announce, ",")
	}
	if len(ips) > 0 && port == 0 {
		return errors.New("announced addresses need a fixed UDP port: set --udp-port or udp_port")
	}
	// One WebRTC API for the test room, the linked browsers and every game
	// room, so all of them use the same UDP port.
	api, err := services.NewWebRTCAPI(port, false)
	if err != nil {
		return err
	}
	// Every stream is tiered: each guest gets its own video track, so the
	// device can send each one the size its screen shows and step one
	// player's quality down while the others keep theirs (video_tiers.go,
	// quality_ladder.go).
	streamCfg := services.StreamConfig{API: api, UDPPort: port, AnnounceIPs: ips, Logger: logger, Tiers: true}
	if *hdCore != "" && *testHD == "auto" {
		return errors.New("--hd-core plays in the test room with --test-room-hd 720p, 1080p or 2160p, not auto")
	}
	if *hdCore != "" {
		library.SetHDCore(*hdCore)
		library.Scan()
	}
	// go-link HD rooms' screen: the flags for this run, else the host's
	// setting (go-link HD's page), read again as each room starts.
	if *hdRoomSize != "" && !slices.Contains(models.HDRoomSizes, *hdRoomSize) {
		return fmt.Errorf("--hd-room-size: %q is not 720p or 1080p", *hdRoomSize)
	}
	hdRoom := func() (scale int, h264 string, kbps int, err error) {
		hd := settings.HD()
		size, codec := hd.RoomSize, hd.Codec
		if *hdRoomSize != "" {
			size = *hdRoomSize
		}
		if *hdRoomCdc != "" {
			codec = *hdRoomCdc
		}
		scale = 2
		if size == models.HD1080p {
			scale = 3
		}
		h264, kbps, err = hdRoomVideo(codec, *hdH264, *hdRoomKbps, scale, runtime.GOOS, encoder.FFmpegPath)
		return scale, h264, kbps, err
	}
	if _, roomH264, roomKbps, err := hdRoom(); err != nil {
		return err
	} else {
		logger.Info("go-link HD rooms", "codec", map[bool]string{true: "h264 " + roomH264, false: "vp8"}[roomH264 != ""], "kbps", roomKbps)
	}
	if *testHD == "auto" {
		// go-link HD picks the size and codec this computer streams at 60 fps (hdAuto)
		if *hdFar == "" {
			return errors.New("--test-room-hd needs --hd-far")
		}
		farImg, err := loadPicture(*hdFar)
		if err != nil {
			return err
		}
		var playImg image.Image
		if *hdPlay != "" {
			if playImg, err = loadPicture(*hdPlay); err != nil {
				return err
			}
		}
		choice, tries := hdAuto(farImg, playImg)
		for _, t := range tries {
			logger.Info("HD check", "size", t.Size, "codec", t.Codec, "h264", t.H264, "max_fps", fmt.Sprintf("%.0f", t.MaxFPS), "p95_ms", fmt.Sprintf("%.1f", t.P95Ms), "fits", t.Fits, "why", t.Why)
		}
		logger.Info("HD choice", "size", choice.Size, "codec", choice.Codec, "h264", choice.H264)
		*testHD, *hdCodec = choice.Size, choice.Codec
		if choice.H264 != "" {
			*hdH264 = choice.H264
		}
	}
	if *testHD != "" {
		streamCfg.EncoderThreads = *hdThreads
		switch *hdCodec {
		case "vp8":
		case "h264":
			if !slices.Contains(encoder.H264Encoders, *hdH264) {
				return fmt.Errorf("--hd-h264: %q is not x264 or videotoolbox", *hdH264)
			}
			if _, err := encoder.FFmpegPath(); err != nil {
				return err
			}
			streamCfg.H264Encoder = *hdH264
		default:
			return fmt.Errorf("--hd-codec: %q is not vp8 or h264", *hdCodec)
		}
	}
	stream, err := services.NewStreamService(streamCfg, ice)
	if err != nil {
		return err
	}
	stream.OnLatency(status.SetPeerLatency)
	pairing.SetLinks(stream)
	// Browsers linked before come back without a code; they must prove
	// their token before they get anything.
	links := services.NewLinkService(services.LinkConfig{
		DeviceID: cfg.DeviceID,
		Links:    cfg.Links,
		Save: func(l []models.Link) error {
			return updateConfig(store, &cfg, func(c *models.Config) { c.Links = l })
		},
		Logger: logger,
	}, status)
	links.SetTransport(stream.SendControl, stream.RemoveViewer)
	pairing.SetAuth(links)
	stream.SetLinkGate(links.Trusted)
	// Each room's telemetry (~/go-link/telemetry.db): runs, samples and
	// events, kept until the room is deleted for good.
	tele, err := telemetry.Open(filepath.Join(base, "telemetry.db"))
	if err != nil {
		logger.Warn("telemetry is off", "err", err)
		tele = nil
	} else if err := tele.EndOpenRuns(); err != nil {
		logger.Warn("telemetry: cannot end the last run", "err", err)
	}
	defer tele.Close()

	var room *services.TestRoomService
	var testManager *services.RoomManager
	if *testRoom {
		// The test pattern room is a permanent diagnostic, like an echo
		// test call in VoIP: it proves web, signalhub, WebRTC and device
		// work end to end.
		room = services.NewTestRoomService(status, stream, hostName(), logger)
		room.SetOpener(opener)
		// Private like every room: the owner opens it from My device and
		// may invite someone to test (a new PIN each run).
		room.Configure("Test pattern", "Test pattern", false)
		room.SetTrusted(links.Trusted)
		room.EnableInvites(nil)
		room.SetPrivate()
		// The test room's telemetry: one run per device run.
		testTele := tele.Room(services.TestRoomID)
		stream.SetTelemetry(testTele, hostTelemetry(status))
		testTele.Start(services.NewRunID(), "Test pattern")
		defer testTele.End("device_stopped")
		manager := services.NewRoomManager(services.RoomManagerConfig{Logger: logger, OnSummary: room.OnSummary, OnLog: services.RoomTeleLog(testTele)}, stream)
		room.SetManager(manager)
		room.SetPicture(cfg.TestRoomPicture)
		testManager = manager
		manager.SetInfo(services.RoomInfo{Title: "Test pattern", Game: "Test pattern", Host: hostName()})
		stream.SetRoomHooks(services.RoomHooks{
			Opened:  manager.Join,
			Message: manager.HandleControl,
			Closed:  manager.Leave,
			PortOf:  manager.PortOf,
		})
		if *testPause {
			var paused atomic.Bool
			stream.PausableTestCard(paused.Load)
			manager.OnPause(paused.Store)
			manager.SetPausable(true)
			manager.OnPauseAsk(func(ev services.PauseAskEvent) {
				stream.SendToLinks(services.RoomPauseAskEvent{PauseAskEvent: ev, ID: services.TestRoomID})
			})
		}
		if *hdCore != "" && *testHD != "" {
			if err := useHDCore(stream, *testHD, *hdCore, filepath.Join(base, "system"), *hdKbps, logger); err != nil {
				return err
			}
			manager.SetInfo(services.RoomInfo{Title: "Test pattern", Game: "go-link HD demo", Host: hostName()})
		} else if *testHD != "" {
			if err := useHDScene(stream, *testHD, *hdFar, *hdPlay, *hdKbps, logger); err != nil {
				return err
			}
		}
		go manager.Run(ctx)
		handlers = append(handlers, room)
	}

	// Game rooms: each one is a game in its own process, and several can
	// run at once. The list lives in device.json.
	systemDir := filepath.Join(base, "system")
	history := services.NewHistoryService(filepath.Join(base, "history.json"))
	// Recordings of game rooms, only for the host (~/go-link/rec).
	recordings := services.NewRecordingService(filepath.Join(base, "rec"), nil)
	history.SetRecordings(recordings)
	probes := saveProbes(library)
	games := services.NewRoomsService(services.RoomsConfig{
		Library: library,
		Status:  status,
		ICE:     ice,
		Stream:  streamCfg,
		HDVideo: func() (string, int) {
			_, h264, kbps, err := hdRoom()
			if err != nil {
				logger.Warn("go-link HD rooms' screen", "err", err)
				return "", 0
			}
			return h264, kbps
		},
		Opener:     opener,
		HostName:   hostName(),
		SavesDir:   filepath.Join(base, "saves"),
		History:    history,
		Telemetry:  tele,
		Host:       hostTelemetry(status),
		Recordings: recordings,
		OnRecording: func(ev services.RecordingEvent) {
			stream.SendToLinks(ev)
		},
		// A player asks the host for a pause: every linked browser shows it.
		OnPauseAsk: func(ev services.RoomPauseAskEvent) {
			stream.SendToLinks(ev)
		},
		MaxRooms:     cfg.MaxRooms,
		VideoQuality: settings.VideoQuality(),
		Trusted:      links.Trusted,
		Rooms:        cfg.Rooms,
		Save: func(list []models.SavedRoom) error {
			return updateConfig(store, &cfg, func(c *models.Config) { c.Rooms = list })
		},
		ProbeSaves: func(ctx context.Context, rom string) (bool, error) {
			if library.IsHD(rom) {
				return true, nil // go-link HD's save states are exact by design
			}
			return probes.SavesWork(ctx, rom)
		},
		NewSource: func(rom, state string, onReady func(libretro.AVInfo)) services.RoomSource {
			upscale, hd := 0, library.IsHD(rom)
			if hd {
				upscale, _, _, _ = hdRoom()
			}
			return services.NewWorkerSource(services.WorkerConfig{
				CorePath: library.CoreFor(rom), RomPath: library.RomPath(rom), SystemDir: systemDir,
				StatePath: state, Upscale: upscale, Native: hd, Language: gameLanguage(cfg.Language), Logger: logger, OnReady: onReady,
			})
		},
		Logger: logger,
	})
	handlers = append(handlers, games)
	// A new video quality (window, website) reaches the running rooms at once.
	settings.OnVideoQuality(func(q string) {
		games.SetVideoQuality(q)
		status.SetVideoQuality(q)
	})

	// go-link HD's free starter games: downloaded once, when a browser
	// first links, each with a room to play it.
	hdGames := services.NewHDGamesService(services.HDGamesConfig{
		Library:  library,
		Status:   status,
		Settings: settings,
		Logger:   logger,
		Offered:  cfg.StarterGames,
		SaveOffered: func() error {
			return updateConfig(store, &cfg, func(c *models.Config) { c.StarterGames = true })
		},
		OpenRoom: func(rom string) error {
			if games.HasRoomFor(rom) {
				return nil
			}
			err := games.Create(services.GameRequest{Rom: rom, Voice: true}, func(services.GameReply) {})
			if errors.Is(err, services.ErrTooManyRooms) {
				// the device already runs its limit: the room waits archived
				return games.AddArchived(rom, true)
			}
			return err
		},
	})

	// Guests see whether the host can answer a request for a pause: a
	// linked browser of the host is connected (or the host is in the room).
	var hostMu sync.Mutex
	hostLinked := false
	hostChanged := func() {
		hostMu.Lock()
		defer hostMu.Unlock()
		on := stream.HasLink()
		if hostLinked == on {
			return
		}
		hostLinked = on
		games.SetHostLinked(on)
		if on {
			// the free starter games, once: downloaded and given a room
			hdGames.OfferStarter(ctx)
		}
		if testManager != nil {
			testManager.SetHostLinked(on)
		}
	}
	links.OnChange(hostChanged)
	stream.OnLinksChanged(hostChanged)

	// ROM uploads from the owner, over WebRTC.
	uploads := services.NewUploadService(library, func(peerID string, r services.FileReply) {
		if b, err := json.Marshal(r); err == nil {
			stream.SendControl(peerID, b)
		}
	})
	// ROM tests (validation level 4): a set the owner sends with purpose
	// rom_test, powered on by a worker process; never the ROM folder.
	romTests := services.NewRomTestService(services.RomTestConfig{
		Dir: filepath.Join(base, "tmp", "romtest"), CorePath: library.CorePath,
		Catalog: library.Catalog, Logger: logger,
	})
	romTests.Cleanup()
	uploads.SetTests(romTests)
	// The game Willy Maker sends (purpose maker), for its own room.
	uploads.SetMaker(library)
	// Recordings to the owner, pulled piece by piece on the same channel.
	downloads := services.NewDownloadService(recordings, stream.SendFiles)
	stream.OnLinkFiles(func(peerID string, isString bool, data []byte) {
		if isString && downloads.Handle(peerID, data) {
			return
		}
		uploads.Handle(peerID, isString, data)
	}, func(peerID string) {
		uploads.Abort(peerID)
		downloads.Abort(peerID)
	})

	// Requests from the owner (a browser linked with the pairing code).
	stream.OnLinkMessage(func(peerID string, data []byte) {
		// Go matches JSON keys in any case and keeps the last one, so a message
		// with "type" and "TYPE" would get past a filter that checked "type"
		// (the website's Willy Maker bridge): such a message is never read.
		if !jsonkeys.Distinct(data) {
			slog.Warn("control message with keys equal but for case dropped", "peer", peerID)
			return
		}
		// auth and unlink first; anything else only from trusted browsers.
		if links.HandleMessage(peerID, data) || !links.Trusted(peerID) {
			return
		}
		// A room's telemetry, only to the owner's linked browsers. Read on
		// its own: its from and to are numbers, which the message below
		// (from: a peer id) would refuse.
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &head) == nil && strings.HasPrefix(head.Type, "telemetry_") {
			go func() {
				if b := answerTelemetry(tele, data); b != nil {
					stream.SendControl(peerID, b)
				}
			}()
			return
		}
		var msg struct {
			Type    string `json:"type"`
			Dir     string `json:"dir"`
			Set     string `json:"set"`
			Kind    string `json:"kind"`
			Size    string `json:"size"`
			ID      string `json:"id"`
			Action  string `json:"action"`
			Name    string `json:"name"`
			From    string `json:"from"`
			Slot    int    `json:"slot"`
			Confirm string `json:"confirm"`
			Accept  bool   `json:"accept"`
			Style   string `json:"style"` // room_action picture
			Bands   string `json:"bands"`
			Quality string `json:"quality"` // set_video_quality
			Frames  int    `json:"frames"`  // rom_test
			// roms_query and roms_get: req is echoed in the reply
			Req    string   `json:"req"`
			Q      string   `json:"q"`
			Filter string   `json:"filter"`
			Sort   string   `json:"sort"`
			Offset int      `json:"offset"`
			Limit  int      `json:"limit"`
			Names  []string `json:"names"`
			services.GameRequest
		}
		if json.Unmarshal(data, &msg) != nil {
			return
		}
		reply := func(r services.GameReply) {
			if b, err := json.Marshal(r); err == nil {
				stream.SendControl(peerID, b)
			}
		}
		var err error
		switch msg.Type {
		case "download_core":
			err = library.DownloadCore(ctx)
		case "get_thumb":
			// A small JPEG of one of the host's thumbnails: "card" for the
			// ROM grid, "mini" for lists.
			w, h := 360, 480
			if msg.Size == "mini" {
				w, h = 96, 128
			}
			kind := thumbnails.Kind(msg.Kind)
			if kind == "" {
				kind = library.ThumbnailKind() // the one the host chose
			}
			res := map[string]any{"type": "thumb", "set": msg.Set, "kind": msg.Kind, "size": msg.Size}
			if b, err := library.Thumbnail(msg.Set, kind, w, h, 90_000); err == nil {
				res["data"] = base64.StdEncoding.EncodeToString(b)
			} else {
				res["missing"] = true
			}
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "roms_query", "roms_get":
			// A page of the ROM library (search, filter, order), or some
			// sets by name: the library is never sent whole (library_index.go).
			res := map[string]any{"type": "roms_page", "req": msg.Req}
			if msg.Type == "roms_query" {
				rev, total, page := library.QueryRoms(services.RomQuery{Q: msg.Q, Filter: msg.Filter, Sort: msg.Sort, Offset: msg.Offset, Limit: msg.Limit})
				res["revision"], res["total"], res["offset"], res["roms"] = rev, total, max(msg.Offset, 0), page
			} else {
				rev, roms := library.GetRoms(msg.Names)
				res["revision"], res["total"], res["offset"], res["roms"] = rev, len(roms), 0, roms
			}
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "rom_test":
			// Power on a set the owner sent on the files channel with
			// purpose rom_test, in a worker process (one test at a time,
			// up to a minute); the set is deleted afterwards.
			id, set, frames := msg.ID, msg.Set, msg.Frames
			go func() {
				res := romTests.Run(ctx, id, set, frames)
				logger.Info("rom test", "set", set, "ok", res.OK, "seconds", res.Seconds)
				if b, err := json.Marshal(res); err == nil {
					stream.SendControl(peerID, b)
				}
			}()
			return
		case "set_roms_dir":
			res := map[string]any{"type": "roms_dir_result", "dir": msg.Dir, "ok": true}
			if err := library.SetDir(msg.Dir); err != nil {
				res["ok"], res["error"] = false, err.Error()
			}
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "set_thumbnails":
			// Which thumbnail everyone sees, and where the device looks for
			// them: the window's Settings, from the linked website. An empty
			// field keeps its value; dir "default" goes back to the default.
			t := settings.Thumbnails()
			if msg.Kind != "" {
				t.Kind = msg.Kind
			}
			switch msg.Dir {
			case "":
			case "default":
				t.Dir = ""
			default:
				t.Dir = msg.Dir
			}
			res := map[string]any{"type": "thumbnails_result", "ok": true}
			if err := settings.SetThumbnails(t); err != nil {
				res["ok"], res["error"] = false, err.Error()
			}
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "set_video_quality":
			// The video quality of game rooms (Settings in the window).
			res := map[string]any{"type": "video_quality_result", "quality": msg.Quality, "ok": true}
			if err := settings.SetVideoQuality(msg.Quality); err != nil {
				res["ok"], res["error"] = false, err.Error()
			}
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "get_history", "clear_history", "delete_history":
			// The history of games, only to the owner's linked browsers.
			// Clearing it (or deleting a game) deletes its recordings too.
			res := map[string]any{"type": "history"}
			switch msg.Type {
			case "clear_history":
				if err := history.Clear(); err != nil {
					logger.Warn("cannot clear the history", "err", err)
				}
			case "delete_history":
				if err := history.Delete(msg.ID); err != nil {
					res["error"] = err.Error()
				}
			}
			res["items"] = history.List()
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "get_recordings", "delete_recording":
			res := map[string]any{"type": "recordings"}
			if msg.Type == "delete_recording" {
				if err := recordings.Delete(msg.ID); err != nil {
					res["error"] = err.Error()
					if code, _ := services.ErrorCode(err); code != "" {
						res["code"] = code
					}
				} else if err := history.ForgetRecording(msg.ID); err != nil {
					logger.Warn("cannot update the history", "err", err)
				}
			}
			res["items"], res["bytes"] = recordings.List(), recordings.Bytes()
			if b, err := json.Marshal(res); err == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "factory_reset":
			// Everything the host made goes; the device keeps only what
			// makes it itself and reachable (see factoryReset). The browser
			// must say so twice: the web asks first, and sends confirm.
			if msg.Confirm != "factory_reset" {
				return
			}
			go func() {
				err := factoryReset(ctx, resetParts{
					games: games, history: history, recordings: recordings, telemetry: tele,
					settings: settings, library: library, romsDir: filepath.Join(base, "roms"),
					hdDir: filepath.Join(base, "hd-games"), hdGames: hdGames,
					config: func(change func(*models.Config)) error { return updateConfig(store, &cfg, change) },
				})
				res := map[string]any{"type": "factory_reset_result", "ok": err == nil}
				if err != nil {
					res["error"] = err.Error()
				}
				if b, jerr := json.Marshal(res); jerr == nil {
					stream.SendControl(peerID, b)
				}
				if err == nil && room != nil {
					room.SetPicture(nil) // device.json's test_room_picture is gone too
				}
				if err == nil {
					// Last: every browser forgets the device (this one too).
					time.Sleep(500 * time.Millisecond)
					links.UnlinkAll()
				}
			}()
			return
		case "create_room":
			err = games.Create(msg.GameRequest, reply)
		case "room_start":
			err = games.Start(msg.ID, msg.From, msg.Slot, reply)
		case "pause_answer":
			// The host's answer to a player's request for a pause (from is
			// the player's peer id, as in pause_asked).
			res := map[string]any{"type": "room_result", "id": msg.ID, "action": "pause_answer", "ok": true}
			if msg.ID == services.TestRoomID && testManager != nil {
				if !testManager.AnswerPause(msg.From, msg.Accept) {
					res["ok"], res["error"] = false, services.ErrRoomState.Error()
				}
			} else if err := games.AnswerPause(msg.ID, msg.From, msg.Accept); err != nil {
				res["ok"], res["error"] = false, err.Error()
			}
			if b, jerr := json.Marshal(res); jerr == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "invite":
			// A new invitation: a PIN good for one person, only for the
			// host's own browsers (this is the linked control channel).
			var pass services.Pass
			if msg.ID == services.TestRoomID && room != nil {
				pass = room.IssuePass()
			} else {
				pass, err = games.Invite(msg.ID)
			}
			res := map[string]any{"type": "invite_pass", "id": msg.ID}
			if err != nil {
				res["error"] = err.Error()
			} else {
				res["pin"], res["expires_at"] = pass.Pin, pass.ExpiresAt
			}
			if b, jerr := json.Marshal(res); jerr == nil {
				stream.SendControl(peerID, b)
			}
			return
		case "room_action", "close_room":
			action := msg.Action
			if msg.Type == "close_room" {
				action = "archive"
			}
			if msg.Type == "close_room" && msg.ID == "" {
				games.Close(reply)
				return
			}
			// The test pattern room ("test"): a new link, and a pause with
			// --test-room-pause.
			if msg.ID == services.TestRoomID && room != nil {
				ok := action == "new_link" || ((action == "pause" || action == "resume") && *testPause) || action == "input_hud_on" || action == "input_hud_off"
				switch action {
				case "new_link":
					room.NewInvite()
				case "input_hud_on", "input_hud_off":
					// The latency test: the beacons of the 4 seats on the card.
					on := action == "input_hud_on"
					if on {
						stream.SetInputHUD(input.MaxLocalPlayers)
					} else {
						stream.SetInputHUD(0)
					}
					if testManager != nil {
						testManager.SetInputHUD(on)
					}
				case "pause", "resume":
					if ok {
						testManager.Pause(action == "pause", "The host")
					}
				case "picture":
					var p *models.RoomPicture
					if p, ok = roomPicture(msg.Style, msg.Bands); ok {
						if err := updateConfig(store, &cfg, func(c *models.Config) { c.TestRoomPicture = p }); err != nil {
							logger.Warn("cannot save the test room's picture", "err", err)
						}
						room.SetPicture(p)
					}
				}
				if b, err := json.Marshal(map[string]any{"type": "room_result", "id": msg.ID, "action": action, "ok": ok}); err == nil {
					stream.SendControl(peerID, b)
				}
				return
			}
			if action == "picture" {
				res := map[string]any{"type": "room_result", "id": msg.ID, "action": action, "ok": true}
				p, ok := roomPicture(msg.Style, msg.Bands)
				err := services.ErrBadPicture
				if ok {
					err = games.SetPicture(msg.ID, p)
				}
				if err != nil {
					res["ok"], res["error"] = false, err.Error()
				}
				if b, jerr := json.Marshal(res); jerr == nil {
					stream.SendControl(peerID, b)
				}
				return
			}
			// Saving and archiving wait for the game: answer off the
			// message loop.
			go func() {
				slot, err := games.Action(ctx, msg.ID, action, msg.Name)
				res := map[string]any{"type": "room_result", "id": msg.ID, "action": action, "ok": err == nil}
				if err != nil {
					res["error"] = err.Error()
					if code, limit := services.ErrorCode(err); code != "" {
						res["code"], res["limit"] = code, limit
					}
				}
				if slot > 0 {
					res["slot"] = slot
				}
				if b, err := json.Marshal(res); err == nil {
					stream.SendControl(peerID, b)
				}
			}()
			return
		default:
			return
		}
		if err != nil {
			logger.Warn("owner request failed", "type", msg.Type, "err", err)
			code, limit := services.ErrorCode(err)
			reply(services.GameReply{Type: "room_error", ID: msg.ID, Rom: msg.Rom, Error: err.Error(), Code: code, Limit: limit})
		}
	})

	if *game != "" && games.Current() == "" {
		if err := games.Create(services.GameRequest{Rom: *game, Public: true, Voice: true}, func(r services.GameReply) {
			logger.Info("game room", "type", r.Type, "room_id", r.RoomID, "error", r.Error)
		}); err != nil {
			return fmt.Errorf("--game %s: %w", *game, err)
		}
	}

	client := signalclient.New(signalclient.Config{URL: target, Logger: logger}, handlers)
	pairing.SetSender(client)
	// The device's data links go through signalhub, or through the local
	// panel's own connection for the panel's browsers.
	router := services.NewRoutingSender()
	router.SetBase(client)
	stream.SetSender(router)
	opener.SetSender(client)
	// Rooms answer the panel's players through the panel's own sockets.
	games.SetSender(router)
	if room != nil {
		room.SetSender(router)
	}
	go games.Run(ctx)
	go func() {
		if err := stream.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("stream stopped", "err", err)
		}
	}()
	metrics := services.NewMetricsService(ctx, status, 2*time.Second, func(st models.Status) {
		stream.SendToLinks(services.NewDeviceStatusMessage(st))
	})
	go metrics.Run(ctx)
	// A newer release shows in the window and on the linked website.
	go services.NewUpdateService(services.UpdateConfig{
		Current: version,
		Logger:  logger,
		OnUpdate: func(u models.UpdateInfo) {
			status.SetUpdate(&u)
		},
	}).Run(ctx)
	// What is being streamed, for the window (once a second).
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				status.SetStream(stream.Stats())
			}
		}
	}()

	// Without a window, the device opens its web panel on the LAN.
	if *headless && !*noPanel {
		// Make the token the first time; the panel reads it from cfg.
		if _, err := panelToken(store, &cfg, false); err != nil {
			return err
		}
		srv := panel.New(panel.Config{
			Addr: *panelAddr, Files: web.Panel(), Logger: logger,
			Token: func() string {
				configMu.Lock()
				defer configMu.Unlock()
				return cfg.PanelToken
			},
			Links: links, Streams: stream, Router: router, Status: status,
			Rooms: panelRooms(room, games),
		})
		// The token itself stays out of the log (logs get shared and stored):
		// the panel asks for it and `device panel token` shows it.
		logger.Info("web panel ready: it asks for the panel token, which `device panel token` shows")
		go func() {
			if err := srv.Run(ctx); err != nil {
				logger.Error("web panel stopped", "addr", *panelAddr, "err", err)
			}
		}()
	}

	go func() {
		if err := client.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("signal client stopped", "err", err)
			cancel()
		}
	}()

	// The window (or, headless, nothing) runs on the main goroutine until
	// the device stops.
	web := cfg.EffectiveWebURL()
	if *webURL != "" {
		web = *webURL
	}
	if !links.Linked() {
		logger.Info("not linked yet: open the website and type the pairing code", "url", strings.TrimRight(web, "/")+"/device")
	}
	runUI(uiOptions{
		ctx:      ctx,
		settings: settings,
		links:    links,
		webURL:   web,
		language: cfg.Language,
		setLanguage: func(id string) error {
			return updateConfig(store, &cfg, func(c *models.Config) { c.Language = id })
		},
		quit:     cancel,
		headless: *headless,
		status:   status,
		library:  library,
		games:    games,
		hdGames:  hdGames,
		logger:   logger,
	})
	logger.Info("shutting down")
	// Save every running game so its room comes back where it was (the
	// games run apart from ctx, so they are still up here).
	shutdownCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	games.Shutdown(shutdownCtx)
	cancel()
	return nil
}

// resolveSignalURL picks the signaling server, in order of precedence:
// the --server-signaling flag (this run only), signal_url in device.json
// (permanent), and finally the project's public server.
func resolveSignalURL(flagValue string, cfg models.Config) (string, error) {
	target := cfg.EffectiveSignalURL()
	source := "signal_url in device.json"
	if flagValue != "" {
		target = flagValue
		source = "--server-signaling"
	}
	if _, err := signalclient.CheckURL(target); err != nil {
		return "", fmt.Errorf("invalid %s: %w", source, err)
	}
	return target, nil
}

// isLoopback reports whether the URL points at this machine.
func isLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostName is the short machine name shown in the lobby.
func hostName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "device"
	}
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}

// roomPicture reads room_action picture's values: both empty clear the
// room's default picture (nil); otherwise both must be known ones.
func roomPicture(style, bands string) (*models.RoomPicture, bool) {
	if style == "" && bands == "" {
		return nil, true
	}
	p := &models.RoomPicture{Style: style, Bands: bands}
	return p, p.Valid()
}

// useHDScene makes the test room stream go-link HD's test scene (T-31).
func useHDScene(stream *services.StreamService, size, far, play string, kbps int, log *slog.Logger) error {
	sz, ok := hdSizes[size]
	if !ok {
		return fmt.Errorf("--test-room-hd: unknown size %q (720p, 1080p or 2160p)", size)
	}
	if far == "" {
		return errors.New("--test-room-hd needs --hd-far")
	}
	farImg, err := loadPicture(far)
	if err != nil {
		return err
	}
	var playImg image.Image
	if play != "" {
		if playImg, err = loadPicture(play); err != nil {
			return err
		}
	}
	if kbps <= 0 {
		kbps = sz.Kbps
	}
	stream.SetBitrate(kbps)
	stream.SetSource(&services.HDSceneSource{Width: sz.W, Height: sz.H, FPS: 60, Far: farImg, Play: playImg})
	log.Info("test room streams the HD scene", "size", size, "kbps", kbps)
	return nil
}

// useHDCore makes the test room play a go-link HD libretro core's built-in
// demo (no content), its 640 x 360 screen enlarged to the HD size.
func useHDCore(stream *services.StreamService, size, core, systemDir string, kbps int, log *slog.Logger) error {
	sz, ok := hdSizes[size]
	if !ok {
		return fmt.Errorf("--test-room-hd: unknown size %q (720p, 1080p or 2160p)", size)
	}
	if _, err := os.Stat(core); err != nil {
		return fmt.Errorf("--hd-core: %w", err)
	}
	if kbps <= 0 {
		kbps = sz.Kbps
	}
	stream.SetBitrate(kbps)
	stream.SetSource(services.NewEmulatorSource(services.EmulatorConfig{
		CorePath:  core,
		SystemDir: systemDir,
		Upscale:   sz.H / 360,
		Native:    true,
		Logger:    log,
	}))
	log.Info("test room plays go-link HD", "core", core, "size", size, "kbps", kbps)
	return nil
}

// hostTelemetry gives each room sample the computer's own numbers: CPU,
// the device's memory and the whole machine's network traffic (its upload
// is what every guest's picture shares).
func hostTelemetry(status *services.StatusService) func() telemetry.Metrics {
	return func() telemetry.Metrics {
		u := status.Snapshot().System.Usage
		return telemetry.Metrics{
			"cpu_pct":       u.CPUPercent,
			"proc_cpu_pct":  u.ProcessCPUPercent,
			"proc_mem_mb":   float64(u.ProcessRSS) / (1 << 20),
			"net_up_kbps":   float64(u.NetSentBps) * 8 / 1000,
			"net_down_kbps": float64(u.NetRecvBps) * 8 / 1000,
		}
	}
}
