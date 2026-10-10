// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/internal/repositories"
	"github.com/lordbasex/go-link/backend-device/internal/services"
	"github.com/lordbasex/go-link/backend-device/pkg/cores"
	"github.com/lordbasex/go-link/backend-device/pkg/ids"
	"golang.org/x/term"
)

// loadConfig opens device.json (created on first run).
func loadConfig(path string) (*repositories.ConfigFile, models.Config, bool, error) {
	if path == "" {
		var err error
		if path, err = repositories.DefaultConfigPath(); err != nil {
			return nil, models.Config{}, false, err
		}
		dir := filepath.Dir(path)
		migrateLegacy(filepath.Join(filepath.Dir(dir), legacyConfigDir), dir)
	}
	store := repositories.NewConfigFile(path)
	cfg, created, err := store.LoadOrCreate()
	return store, cfg, created, err
}

// Folders of the project's former name, MAME WebRTC.
const (
	legacyConfigDir = "mame-webrtc"
	legacyDataDir   = "MAME-WebRTC"
)

// migrateLegacy moves a folder of the former name to the new one, once:
// only when the new one does not exist yet.
func migrateLegacy(oldPath, newPath string) bool {
	if _, err := os.Stat(newPath); err == nil {
		return false
	}
	if _, err := os.Stat(oldPath); err != nil {
		return false
	}
	return os.Rename(oldPath, newPath) == nil
}

// dataDir is where the device keeps its cores, system files and default
// ROM folder: ~/go-link. It does not move with the ROM folder, which
// can be anywhere on the disk.
func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "go-link"), nil
}

// openLibrary builds the status and the ROM library shared by the device
// and the CLI commands. cfg is updated (and saved) when the ROM folder
// changes.
func openLibrary(store *repositories.ConfigFile, cfg *models.Config, target, corePath string, logger *slog.Logger) (*services.StatusService, *services.LibraryService, *services.SettingsService, string, error) {
	base, err := dataDir()
	if err != nil {
		return nil, nil, nil, "", err
	}
	// Cores, ROMs and system files move with the project's new name, and
	// a ROM folder inside the old one follows them.
	old := filepath.Join(filepath.Dir(base), legacyDataDir)
	if migrateLegacy(old, base) && strings.HasPrefix(cfg.RomsDir, old+string(filepath.Separator)) {
		moved := filepath.Join(base, strings.TrimPrefix(cfg.RomsDir, old+string(filepath.Separator)))
		if err := updateConfig(store, cfg, func(c *models.Config) { c.RomsDir = moved }); err != nil {
			logger.Warn("cannot save the moved ROM folder", "err", err)
		}
	}
	// ROM folder: the host's own disk. ROMs never leave it.
	if cfg.RomsDir == "" {
		cfg.RomsDir = filepath.Join(base, "roms")
		if err := store.Save(*cfg); err != nil {
			logger.Warn("cannot save the default ROM folder", "err", err)
		}
	}
	status := services.NewStatusService(cfg.DeviceID, version, target, cfg.RomsDir)
	library := services.NewLibraryService(cfg.RomsDir, status, logger)
	// the game Willy Maker sends (purpose maker), in its own folder
	library.SetMakerDir(filepath.Join(base, "maker"))
	coresDir := filepath.Join(base, "cores")
	if corePath != "" {
		coresDir = filepath.Dir(corePath)
	}
	coreURL, err := cores.URL(cores.BuildbotURL, cores.DefaultCore, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		logger.Warn("no prebuilt emulator core for this system", "err", err)
	}
	library.SetCore(coresDir, coreURL)
	// The host's own thumbnails (one folder per emulator) and how to show
	// them, from Settings.
	settings := services.NewSettingsService(library, cfg.Thumbnails, filepath.Join(base, "thumbnails", "MAME"), func(t models.ThumbnailSettings) error {
		return updateConfig(store, cfg, func(c *models.Config) { c.Thumbnails = t })
	})
	settings.UseVideoQuality(cfg.VideoQuality, func(q string) error {
		return updateConfig(store, cfg, func(c *models.Config) { c.VideoQuality = q })
	})
	status.SetVideoQuality(settings.VideoQuality())
	// go-link HD's games, in their own folder (made the first time).
	settings.UseHD(cfg.HD, func(h models.HDSettings) error {
		return updateConfig(store, cfg, func(c *models.Config) { c.HD = h })
	})
	hdDir := cfg.HD.GamesDir
	if hdDir == "" {
		hdDir = filepath.Join(base, "hd-games")
		if err := os.MkdirAll(hdDir, 0o755); err != nil {
			logger.Warn("cannot make go-link HD's games folder", "err", err)
		}
	}
	library.UseHDDir(hdDir)
	library.OnHDDirChange(func(dir string) {
		if err := settings.SetHDGamesDir(dir); err != nil {
			logger.Warn("cannot save go-link HD's games folder", "err", err)
		}
	})
	library.OnDirChange(func(dir string) {
		if err := updateConfig(store, cfg, func(c *models.Config) { c.RomsDir = dir }); err != nil {
			logger.Warn("cannot save the ROM folder", "err", err)
		}
	})
	library.Scan()
	return status, library, settings, base, nil
}

// configMu serializes changes to device.json: the ROM folder and the
// linked browsers are saved from different goroutines.
var configMu sync.Mutex

// updateConfig applies a change to cfg and saves it.
func updateConfig(store *repositories.ConfigFile, cfg *models.Config, change func(*models.Config)) error {
	configMu.Lock()
	defer configMu.Unlock()
	change(cfg)
	return store.Save(*cfg)
}

// maxLogSize is when the log file starts again (the old one is kept as .1).
const maxLogSize = 5 << 20

// logOutput is where the log goes: the terminal when there is one, else
// (the app opened from the Finder or the Start menu) ~/go-link/logs/device.log,
// so the pairing code and errors are not lost.
func logOutput() (io.Writer, func()) {
	// A real terminal, not /dev/null (where macOS sends an app's output).
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr, func() {}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Stderr, func() {}
	}
	dir := filepath.Join(home, "go-link", "logs")
	if os.MkdirAll(dir, 0o700) != nil {
		return os.Stderr, func() {}
	}
	path := filepath.Join(dir, "device.log")
	if st, err := os.Stat(path); err == nil && st.Size() > maxLogSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return os.Stderr, func() {}
	}
	return io.MultiWriter(f, os.Stderr), func() { _ = f.Close() }
}

// panelToken returns the local web panel's token, making one (a UUID v4)
// the first time or when renew is set.
func panelToken(store *repositories.ConfigFile, cfg *models.Config, renew bool) (string, error) {
	configMu.Lock()
	token := cfg.PanelToken
	configMu.Unlock()
	if token != "" && !renew && ids.ValidUUID(token) {
		return token, nil
	}
	token = ids.NewUUIDv4()
	if err := updateConfig(store, cfg, func(c *models.Config) { c.PanelToken = token }); err != nil {
		return "", fmt.Errorf("cannot save the panel token: %w", err)
	}
	return token, nil
}

// wantsHeadless reports whether there is no desktop to show a tray icon
// on, like a Raspberry Pi without a screen.
func wantsHeadless() bool {
	return runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

// localRooms lets the local panel's players into the device's rooms: the
// test pattern room and the game rooms.
type localRooms struct {
	test  *services.TestRoomService
	games *services.RoomsService
	services.Dispatcher
}

// panelRooms builds the panel's view of the rooms; test may be nil.
func panelRooms(test *services.TestRoomService, games *services.RoomsService) *localRooms {
	r := &localRooms{test: test, games: games, Dispatcher: services.Dispatcher{games}}
	if test != nil {
		r.Dispatcher = append(r.Dispatcher, test)
	}
	return r
}

// Find implements panel.Rooms.
func (r *localRooms) Find(roomID, invite, code string) string {
	if r.test != nil {
		if id := r.test.Matches(roomID, invite, code); id != "" {
			return id
		}
	}
	return r.games.Find(roomID, invite, code)
}

// saveProbes tests, once per game and core, whether the emulator saves a
// game whole (services.ProbeSaves). The answers live next to the core.
func saveProbes(library *services.LibraryService) *services.SaveProbeCache {
	return services.NewSaveProbeCache(
		filepath.Join(filepath.Dir(library.CorePath()), "saves.json"),
		library.CorePath,
		func(ctx context.Context, corePath, rom string) (bool, error) {
			// Chips some cores do not save must be in the save (game list).
			var needs []string
			if cat := library.Catalog(); cat != nil {
				if g := cat.Games[rom]; g != nil {
					needs = g.SaveModules
				}
			}
			return services.ProbeSaves(ctx, services.SaveProbeConfig{CorePath: corePath, RomPath: library.RomPath(rom), Needs: needs})
		},
	)
}
