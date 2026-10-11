// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/libretro"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
	"github.com/lordbasex/go-link-console/backend-device/pkg/signalclient"
)

// hubSender plays signalhub for the rooms: every room_open is answered
// with a new room id through the device's opener.
type hubSender struct {
	mu     sync.Mutex
	opener *services.RoomOpener
	n      int
	closed []string
}

func (h *hubSender) Send(env signalclient.Envelope) error {
	switch env.Type {
	case signalclient.TypeRoomOpen:
		h.mu.Lock()
		h.n++
		id := "room-" + string(rune('0'+h.n))
		h.mu.Unlock()
		go h.opener.OnMessage(signalclient.Envelope{Type: signalclient.TypeRoomOpened, RoomID: id})
	case signalclient.TypeRoomClose:
		h.mu.Lock()
		h.closed = append(h.closed, env.RoomID)
		h.mu.Unlock()
	}
	return nil
}

// buildDevice compiles the device binary: game rooms run their emulator
// as "device emulate" child processes.
func buildDevice(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go tool is needed to build the device")
	}
	bin := filepath.Join(t.TempDir(), "device")
	cmd := exec.Command("go", "build", "-o", bin, "../cmd/device")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build device: %v\n%s", err, out)
	}
	return bin
}

// TestGameRoomsWithRealCore needs the mame2003-plus core and the robby
// and looping ROM sets in ~/go-link; it is skipped otherwise.
func TestGameRoomsWithRealCore(t *testing.T) {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "go-link")
	romsDir := filepath.Join(base, "roms")
	coresDir := filepath.Join(base, "cores")
	if _, err := os.Stat(filepath.Join(coresDir, cores.FileName(cores.DefaultCore, runtime.GOOS))); err != nil {
		t.Skip("mame2003-plus core not installed")
	}
	for _, rom := range []string{"robby", "looping"} {
		if _, err := os.Stat(filepath.Join(romsDir, rom+".zip")); err != nil {
			t.Skip(rom + ".zip not in the ROM folder")
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	status := services.NewStatusService("7f3c2a10-1b2c-4d3e-8f90-a1b2c3d4e5f6", "test", "ws://x", romsDir)
	bin := buildDevice(t)
	systemDir := filepath.Join(t.TempDir(), "system") // made first, so it is removed last

	newRooms := func(library *services.LibraryService, saves string) (*services.RoomsService, *hubSender) {
		opener := services.NewRoomOpener()
		hub := &hubSender{opener: opener}
		opener.SetSender(hub)
		rooms := services.NewRoomsService(services.RoomsConfig{
			Library: library, Status: status, ICE: services.NewICEStore(),
			Stream: services.StreamConfig{IncludeLoopback: true, Logger: logger},
			Opener: opener, HostName: "test", SavesDir: saves,
			NewSource: func(rom, state string, onReady func(libretro.AVInfo)) services.RoomSource {
				return services.NewWorkerSource(services.WorkerConfig{
					Exe: bin, CorePath: library.CorePath(), RomPath: library.RomPath(rom),
					SystemDir: systemDir, StatePath: state, Logger: logger, OnReady: onReady,
				})
			},
			Logger: logger,
		})
		rooms.SetSender(hub)
		rooms.OnConnect(signalclient.Envelope{})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			rooms.Shutdown(context.Background())
			cancel()
		})
		go rooms.Run(ctx)
		return rooms, hub
	}
	wait := func(replies chan services.GameReply, what string) services.GameReply {
		t.Helper()
		select {
		case r := <-replies:
			return r
		case <-time.After(30 * time.Second):
			t.Fatalf("no reply for %s", what)
			return services.GameReply{}
		}
	}

	// With the core's game list, an incompatible set is refused before
	// anything is loaded.
	if _, err := os.Stat(filepath.Join(coresDir, romcheck.FileName)); err == nil {
		checked := services.NewLibraryService(romsDir, status, logger)
		checked.SetCore(coresDir, "")
		checked.RefreshCatalog(context.Background()) // as the device does at startup
		rooms, _ := newRooms(checked, t.TempDir())
		if err := rooms.Create(services.GameRequest{Rom: "looping"}, func(services.GameReply) {}); err == nil || !strings.Contains(err.Error(), "files missing") {
			t.Fatalf("looping with the game list: %v", err)
		}
	}

	// Without the game list the core itself finds out: a cores folder
	// that only links the core.
	bare := t.TempDir()
	name := cores.FileName(cores.DefaultCore, runtime.GOOS)
	if err := os.Symlink(filepath.Join(coresDir, name), filepath.Join(bare, name)); err != nil {
		t.Fatal(err)
	}
	library := services.NewLibraryService(romsDir, status, logger)
	library.SetCore(bare, "")
	library.Scan()
	saves := t.TempDir()
	rooms, hub := newRooms(library, saves)

	replies := make(chan services.GameReply, 4)
	if err := rooms.Create(services.GameRequest{Rom: "robby", Public: true}, func(r services.GameReply) { replies <- r }); err != nil {
		t.Fatal(err)
	}
	r := wait(replies, "robby")
	// Without the core's game list there is no catalog title.
	if r.Type != "room_created" || r.Title != "robby" || r.RoomID == "" || r.ID == "" {
		t.Fatalf("robby: %+v", r)
	}
	id := r.ID

	// Save a game and archive: the emulator writes real save states.
	slot, err := rooms.Action(context.Background(), id, "save", "start")
	if err != nil || slot != 1 {
		t.Fatalf("save: %d %v", slot, err)
	}
	if _, err := rooms.Action(context.Background(), id, "archive", ""); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"slot-1.state", "auto.state"} {
		if fi, err := os.Stat(filepath.Join(saves, id, f)); err != nil || fi.Size() == 0 {
			t.Fatalf("%s: %v", f, err)
		}
	}
	// Turn it on again from the saved game.
	if err := rooms.Start(id, "slot", 1, func(r services.GameReply) { replies <- r }); err != nil {
		t.Fatal(err)
	}
	if r := wait(replies, "restart"); r.Type != "room_created" || r.ID != id {
		t.Fatalf("restart: %+v", r)
	}

	// A ROM the core cannot run: error, and the room is archived.
	if err := rooms.Create(services.GameRequest{Rom: "looping"}, func(r services.GameReply) { replies <- r }); err != nil {
		t.Fatal(err)
	}
	if r := wait(replies, "looping"); r.Type != "room_error" || r.Rom != "looping" {
		t.Fatalf("looping: %+v", r)
	}
	for _, room := range rooms.List() {
		if room.Rom == "looping" && room.State != models.RoomArchived {
			t.Fatalf("failed room %+v", room)
		}
	}
	hub.mu.Lock()
	closed := len(hub.closed)
	hub.mu.Unlock()
	if closed == 0 {
		t.Fatal("archiving must close the signalhub room")
	}

	if err := rooms.Create(services.GameRequest{Rom: "nope"}, nil); err == nil {
		t.Fatal("unknown ROM accepted")
	}
}
