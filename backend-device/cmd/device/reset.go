// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/internal/telemetry"
	"github.com/lordbasex/go-link-console/backend-device/pkg/instancelock"
)

// resetParts are what a factory reset of the running device touches.
type resetParts struct {
	games      *services.RoomsService
	history    *services.HistoryService
	recordings *services.RecordingService
	telemetry  *telemetry.Store
	settings   *services.SettingsService
	library    *services.LibraryService
	romsDir    string // the default ROM folder
	hdDir      string // go-link HD's default games folder
	hdGames    *services.HDGamesService
	config     func(change func(*models.Config)) error
}

// factoryReset puts the running device back as it was installed: every
// room stops (without saving) and is forgotten with its saved games, the
// history and every recording are deleted, and the settings go back to
// their defaults (models.Config.FactoryDefaults). The caller unlinks the
// browsers last. The host's own files stay: ROMs, thumbnails, the
// emulator core and the logs are never deleted.
func factoryReset(ctx context.Context, p resetParts) error {
	var errs []error
	p.games.Reset(ctx)
	if err := p.history.Clear(); err != nil {
		errs = append(errs, err)
	}
	p.recordings.DeleteAll()
	if err := p.telemetry.DeleteAll(); err != nil {
		errs = append(errs, err)
	}
	if err := p.settings.SetThumbnails(models.ThumbnailSettings{}); err != nil {
		errs = append(errs, err)
	}
	if err := p.settings.SetVideoQuality(models.DefaultVideoQuality); err != nil {
		errs = append(errs, err)
	}
	if err := os.MkdirAll(p.romsDir, 0o755); err != nil {
		errs = append(errs, err)
	} else if err := p.library.SetDir(p.romsDir); err != nil {
		errs = append(errs, err)
	}
	if err := p.settings.SetHDScreen(models.HD720p, models.HDAuto); err != nil {
		errs = append(errs, err)
	}
	if p.hdDir != "" {
		if err := os.MkdirAll(p.hdDir, 0o755); err != nil {
			errs = append(errs, err)
		} else if err := p.library.SetHDDir(p.hdDir); err != nil {
			errs = append(errs, err)
		}
	}
	if p.hdGames != nil {
		p.hdGames.ResetOffered() // offered again on the next link
	}
	if err := p.config(func(c *models.Config) {
		*c = c.FactoryDefaults()
		c.RomsDir = p.romsDir
	}); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// openRecordings opens the recordings and the history for the CLI.
func openRecordings() (*services.RecordingService, *services.HistoryService, string, error) {
	base, err := dataDir()
	if err != nil {
		return nil, nil, "", err
	}
	recs := services.NewRecordingService(filepath.Join(base, "rec"), nil)
	history := services.NewHistoryService(filepath.Join(base, "history.json"))
	history.SetRecordings(recs)
	return recs, history, base, nil
}

func cmdRecList(args []string) error {
	fs, _ := newFlags("rec list")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	recs, _, _, err := openRecordings()
	if err != nil {
		return err
	}
	list := recs.List()
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}
	if len(list) == 0 {
		fmt.Println("No recordings in", recs.Dir())
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tROOM\tSTARTED\tLENGTH\tSIZE\tTRACKS")
	for _, r := range list {
		length := (time.Duration(r.DurationMS) * time.Millisecond).Round(time.Second)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.1f MB\t%s\n", r.ID, r.Room, r.StartedAt.Local().Format("2006-01-02 15:04"), length, float64(r.Size)/1e6, strings.Join(r.Tracks, ","))
	}
	return w.Flush()
}

func cmdRecRm(args []string) error {
	fs, _ := newFlags("rec rm")
	all := fs.Bool("all", false, "delete every recording")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	recs, history, _, err := openRecordings()
	if err != nil {
		return err
	}
	if *all {
		fmt.Println("Deleted", recs.DeleteAll(), "recordings")
		return nil
	}
	if fs.NArg() == 0 {
		return errors.New("which recording? give its ID (device rec list) or --all")
	}
	for _, id := range fs.Args() {
		if err := recs.Delete(id); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		if err := history.ForgetRecording(id); err != nil {
			return err
		}
		fmt.Println("Deleted", id)
	}
	return nil
}

// cmdReset is the factory reset for a stopped device (a running one gets
// it from its linked website).
func cmdReset(args []string) error {
	fs, config := newFlags("reset")
	yes := fs.Bool("yes", false, "really reset: rooms, saved games, history, recordings, linked browsers and settings are deleted")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if !*yes {
		return errors.New("this deletes every room, saved game, recording, the history, the linked browsers and the settings (your ROMs, thumbnails and the emulator stay): run it again with --yes")
	}
	store, cfg, _, err := loadConfig(*config)
	if err != nil {
		return err
	}
	lock, err := instancelock.Acquire(filepath.Join(filepath.Dir(store.Path()), "device.lock"))
	if err != nil {
		return errors.New("the device is running: reset it from its website (My device), or stop it first")
	}
	defer lock.Release()
	recs, history, base, err := openRecordings()
	if err != nil {
		return err
	}
	var errs []error
	errs = append(errs, history.Clear())
	recs.DeleteAll()
	errs = append(errs, os.RemoveAll(filepath.Join(base, "saves")))
	for _, f := range []string{"telemetry.db", "telemetry.db-wal", "telemetry.db-shm"} {
		if err := os.Remove(filepath.Join(base, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	errs = append(errs, updateConfig(store, &cfg, func(c *models.Config) { *c = c.FactoryDefaults() }))
	if err := errors.Join(errs...); err != nil {
		return err
	}
	fmt.Println("The device is back to its first run. Your ROMs, thumbnails and the emulator were kept.")
	return nil
}
