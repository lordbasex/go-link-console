// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/pkg/glhd"
	"github.com/lordbasex/go-link/backend-device/pkg/hdgames"
)

// ErrUnknownGame is a starter game name that is not in hdgames.Starter.
var ErrUnknownGame = errors.New("there is no starter game with that name")

// HDGamesConfig configures HDGamesService.
type HDGamesConfig struct {
	Library  *LibraryService
	Status   *StatusService
	Settings *SettingsService
	Client   *http.Client // nil: a default client
	Logger   *slog.Logger
	// Offered tells whether the starter games were offered already, and
	// SaveOffered records it in device.json.
	Offered     bool
	SaveOffered func() error
	// OpenRoom makes a game room for a starter game just downloaded
	// (nil: none, as in the CLI).
	OpenRoom func(rom string) error
	// Games are the starter games (default hdgames.Starter; tests).
	Games []hdgames.Game
}

// HDGamesService downloads go-link HD's free starter games into its games
// folder: by itself once, the first time a browser links (each in the
// size of the device's HD rooms, and a room to play it), and whenever the
// host asks from go-link HD's page or the CLI. A game the host deletes is
// never downloaded again by itself.
type HDGamesService struct {
	cfg HDGamesConfig

	mu       sync.Mutex
	offered  bool
	offering bool
	jobs     map[string]*hdJob // running downloads, by game
	errs     map[string]string // the last failed download, by game
	lastPub  time.Time
}

type hdJob struct {
	size        string
	done, total int64
}

// NewHDGamesService builds the service and publishes its state.
func NewHDGamesService(cfg HDGamesConfig) *HDGamesService {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Games == nil {
		cfg.Games = hdgames.Starter
	}
	h := &HDGamesService{cfg: cfg, offered: cfg.Offered, jobs: map[string]*hdJob{}, errs: map[string]string{}}
	// a new screen setting shows at once
	cfg.Settings.OnChange(h.Publish)
	h.Publish()
	return h
}

// Publish sends go-link HD's screen settings and the starter games' state
// to the status (the window, the linked browsers).
func (h *HDGamesService) Publish() {
	hd := h.cfg.Settings.HD()
	st := models.HDStatus{RoomSize: hd.RoomSize, Codec: hd.Codec, Starter: []models.StarterGame{}}
	h.mu.Lock()
	for _, g := range h.cfg.Games {
		sg := models.StarterGame{Name: g.Name, Title: g.Title, Error: h.errs[g.Name]}
		if p := h.cfg.Library.HDPackage(g.Name); p != "" {
			sg.Installed = true
			if m, err := glhd.Read(p); err == nil {
				sg.Size, sg.Version = m.Resolution, m.Version
			}
			if hdgames.Older(sg.Version, g.Version) {
				sg.Update = g.Version
			}
		}
		if j := h.jobs[g.Name]; j != nil {
			sg.Downloading, sg.Done, sg.Total = j.size, j.done, j.total
		}
		st.Starter = append(st.Starter, sg)
	}
	h.mu.Unlock()
	h.cfg.Status.SetHD(st)
}

// Download fetches a starter game in a screen size (720p or 1080p) into
// the HD games folder, in the background.
func (h *HDGamesService) Download(ctx context.Context, name, size string) error {
	g, err := h.start(name, size)
	if err != nil {
		return err
	}
	go h.fetch(ctx, g, size)
	return nil
}

// DownloadWait is Download, waiting until it ends (for the CLI).
func (h *HDGamesService) DownloadWait(ctx context.Context, name, size string) error {
	g, err := h.start(name, size)
	if err != nil {
		return err
	}
	return h.fetch(ctx, g, size)
}

func (h *HDGamesService) start(name, size string) (hdgames.Game, error) {
	i := slices.IndexFunc(h.cfg.Games, func(g hdgames.Game) bool { return g.Name == name })
	if i < 0 {
		return hdgames.Game{}, ErrUnknownGame
	}
	g := h.cfg.Games[i]
	if _, ok := g.Files[size]; !ok {
		return g, ErrBadSetting
	}
	if h.cfg.Library.HDDir() == "" {
		return g, ErrBadFolder
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.jobs[name] != nil {
		return g, ErrDownloadRunning
	}
	h.jobs[name] = &hdJob{size: size, total: g.Files[size].Size}
	delete(h.errs, name)
	return g, nil
}

func (h *HDGamesService) fetch(ctx context.Context, g hdgames.Game, size string) error {
	h.Publish()
	h.cfg.Logger.Info("downloading a go-link HD game", "game", g.Name, "size", size)
	_, err := hdgames.Download(ctx, h.cfg.Client, g, size, h.cfg.Library.HDDir(), func(done, total int64) {
		h.mu.Lock()
		if j := h.jobs[g.Name]; j != nil {
			j.done, j.total = done, total
		}
		// a few repaints a second, not one per piece
		now := time.Now()
		due := now.Sub(h.lastPub) >= 250*time.Millisecond
		if due {
			h.lastPub = now
		}
		h.mu.Unlock()
		if due {
			h.Publish()
		}
	})
	h.mu.Lock()
	delete(h.jobs, g.Name)
	if err != nil {
		h.errs[g.Name] = err.Error()
	}
	h.mu.Unlock()
	if err != nil {
		h.cfg.Logger.Warn("go-link HD game download failed", "game", g.Name, "err", err)
	} else {
		h.cfg.Logger.Info("go-link HD game installed", "game", g.Name, "size", size)
	}
	h.cfg.Library.Scan()
	h.Publish()
	return err
}

// ResetOffered forgets that the starter games were offered (a factory
// reset): the next link offers them again.
func (h *HDGamesService) ResetOffered() {
	h.mu.Lock()
	h.offered = false
	h.mu.Unlock()
}

// OfferStarter runs once per device (until a factory reset): each starter
// game is downloaded in the size of the HD rooms, unless it is already in
// a folder, and gets a room to play it. A failure (no network) leaves it
// for the next time a browser links or the device starts linked.
func (h *HDGamesService) OfferStarter(ctx context.Context) {
	h.mu.Lock()
	if h.offered || h.offering {
		h.mu.Unlock()
		return
	}
	h.offering = true
	h.mu.Unlock()
	go func() {
		ok := h.offer(ctx)
		h.mu.Lock()
		h.offering = false
		if ok {
			h.offered = true
		}
		h.mu.Unlock()
		if ok && h.cfg.SaveOffered != nil {
			if err := h.cfg.SaveOffered(); err != nil {
				h.cfg.Logger.Warn("cannot save that the starter games were offered", "err", err)
			}
		}
	}()
}

func (h *HDGamesService) offer(ctx context.Context) bool {
	if !h.engineInstalled() {
		return false // no engine to play them: offer them on a device that has it
	}
	size := h.cfg.Settings.HD().RoomSize
	for _, g := range h.cfg.Games {
		if h.cfg.Library.HDPackage(g.Name) == "" {
			if _, err := h.start(g.Name, size); err != nil {
				return false
			}
			if err := h.fetch(ctx, g, size); err != nil {
				return false
			}
		}
		if h.cfg.OpenRoom != nil {
			if err := h.cfg.OpenRoom(g.Name); err != nil {
				h.cfg.Logger.Warn("cannot open the starter game's room", "game", g.Name, "err", err)
			}
		}
	}
	return true
}

// engineInstalled reports whether go-link HD's engine is on the device.
func (h *HDGamesService) engineInstalled() bool {
	_, err := os.Stat(h.cfg.Library.HDCorePath())
	return err == nil
}
