// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/lordbasex/go-link/backend-device/internal/models"
	"github.com/lordbasex/go-link/backend-device/internal/services"
)

// cmdHDDir shows or changes go-link HD's games folder.
func cmdHDDir(args []string) error {
	fs, config := newFlags("hd dir")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fmt.Println(env.library.HDDir())
		return nil
	}
	dir := fs.Arg(0)
	if dir == "default" {
		base, err := dataDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(base, "hd-games")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return err
	}
	if err := env.library.SetHDDir(dir); err != nil {
		return err
	}
	n := 0
	if lib := env.status.Snapshot().Library; lib != nil {
		n = lib.HDGames
	}
	fmt.Printf("go-link HD's games folder set to %s (%d games). A running device picks it up on its next start.\n", dir, n)
	return nil
}

// cmdHDScreen shows or changes go-link HD rooms' screen.
func cmdHDScreen(args []string) error {
	fs, config := newFlags("hd screen")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	return hdScreen(os.Stdout, env.settings, fs.Args())
}

// hdScreen shows or sets go-link HD rooms' size and codec, in any order
// (device hd screen 1080p, device hd screen vp8 720p).
func hdScreen(w io.Writer, settings *services.SettingsService, args []string) error {
	hd := settings.HD()
	if len(args) == 0 {
		fmt.Fprintf(w, "size %s, codec %s\n", hd.RoomSize, hd.Codec)
		return nil
	}
	size, codec := hd.RoomSize, hd.Codec
	for _, a := range args {
		a = strings.ToLower(a)
		switch {
		case slices.Contains(models.HDRoomSizes, a):
			size = a
		case slices.Contains(models.HDCodecs, a):
			codec = a
		default:
			return fmt.Errorf("unknown value %q (sizes: 720p, 1080p; codecs: auto, vp8)", a)
		}
	}
	if err := settings.SetHDScreen(size, codec); err != nil {
		return err
	}
	fmt.Fprintf(w, "go-link HD rooms: size %s, codec %s. Rooms started from now on use it (a running device picks it up on its next start).\n", size, codec)
	return nil
}

// openHDGames is the starter games' service for the CLI.
func openHDGames(env *cliEnv) *services.HDGamesService {
	return services.NewHDGamesService(services.HDGamesConfig{Library: env.library, Status: env.status, Settings: env.settings, Offered: true})
}

// cmdHDGames lists go-link HD's starter games and whether they are in the
// games folder.
func cmdHDGames(args []string) error {
	fs, config := newFlags("hd games")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	openHDGames(env)
	hd := env.status.Snapshot().HD
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "GAME\tTITLE\tINSTALLED")
	for _, g := range hd.Starter {
		state := "no"
		if g.Installed {
			state = "yes"
			if g.Size != "" {
				state += " (" + g.Size + ")"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", g.Name, g.Title, state)
	}
	return tw.Flush()
}

// cmdHDDownload downloads a starter game into the games folder.
func cmdHDDownload(args []string) error {
	fs, config := newFlags("hd download")
	size := fs.String("size", "", "screen size: 720p or 1080p (default: go-link HD rooms' size)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: device hd download GAME [--size 720p|1080p]")
	}
	env, err := openEnv(*config)
	if err != nil {
		return err
	}
	if *size == "" {
		*size = env.settings.HD().RoomSize
	}
	ctx, stop := interruptible()
	defer stop()
	fmt.Printf("Downloading %s (%s) into %s…\n", fs.Arg(0), *size, env.library.HDDir())
	if err := openHDGames(env).DownloadWait(ctx, fs.Arg(0), *size); err != nil {
		if errors.Is(err, services.ErrBadSetting) {
			return fmt.Errorf("unknown size %q (use 720p or 1080p)", *size)
		}
		return err
	}
	fmt.Println("Done. Create its room on the website (New game).")
	return nil
}
