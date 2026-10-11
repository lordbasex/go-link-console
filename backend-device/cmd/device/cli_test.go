// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"bytes"
	"flag"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/internal/services"
)

func TestThumbnailReport(t *testing.T) {
	lib := models.Library{Dir: "/roms", ThumbnailsDir: "/thumbs/MAME", Roms: []models.RomInfo{
		{Name: "galaga", Thumbs: models.Thumbs{Boxart: true, Title: true, Snap: true}},
		{Name: "robby", Thumbs: models.Thumbs{Snap: true}},
		{Name: "witchgme"},
	}}
	r := thumbnailReport(lib)
	if r.Sets != 3 || r.Boxart != 1 || r.Title != 1 || r.Snap != 2 {
		t.Fatalf("report = %+v", r)
	}
	if len(r.WithoutAny) != 1 || r.WithoutAny[0] != "witchgme" {
		t.Fatalf("without = %v", r.WithoutAny)
	}
	var out bytes.Buffer
	printThumbReport(&out, r)
	for _, want := range []string{"Thumbnails in /thumbs/MAME for the 3 sets in /roms:", "  Snap    2 / 3\n", "Sets without any thumbnail (1):\n  witchgme\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	printThumbReport(&out, thumbnailReport(models.Library{Roms: lib.Roms[:1]}))
	if !strings.Contains(out.String(), "Every set has at least one thumbnail.") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestThumbnailsDirAndKind(t *testing.T) {
	var saved models.ThumbnailSettings
	def := filepath.Join(t.TempDir(), "thumbnails", "MAME")
	settings := services.NewSettingsService(nil, models.ThumbnailSettings{}, def, func(ts models.ThumbnailSettings) error {
		saved = ts
		return nil
	})
	var out bytes.Buffer
	if err := thumbnailsDir(&out, settings, nil); err != nil || out.String() != def+"\n" {
		t.Fatalf("dir = %q, %v", out.String(), err)
	}
	dir := t.TempDir()
	if err := thumbnailsDir(&out, settings, []string{dir}); err != nil || saved.Dir != dir {
		t.Fatalf("set dir: %v, saved %+v", err, saved)
	}
	if err := thumbnailsDir(&out, settings, []string{filepath.Join(dir, "missing")}); err == nil || !strings.Contains(err.Error(), "not an existing folder") {
		t.Fatalf("missing folder: %v", err)
	}
	if err := thumbnailsDir(&out, settings, []string{"default"}); err != nil || saved.Dir != "" || settings.ThumbnailsDir() != def {
		t.Fatalf("default: %v, saved %+v", err, saved)
	}

	out.Reset()
	if err := thumbnailsKind(&out, settings, nil); err != nil || out.String() != "boxart\n" {
		t.Fatalf("kind = %q, %v", out.String(), err)
	}
	if err := thumbnailsKind(&out, settings, []string{"Snap"}); err != nil || saved.Kind != "snap" {
		t.Fatalf("set kind: %v, saved %+v", err, saved)
	}
	if err := thumbnailsKind(&out, settings, []string{"cover"}); err == nil || !strings.Contains(err.Error(), "use boxart, title or snap") {
		t.Fatalf("bad kind: %v", err)
	}
}

func TestVideoQualityCommand(t *testing.T) {
	settings := services.NewSettingsService(nil, models.ThumbnailSettings{}, t.TempDir(), nil)
	saved := ""
	settings.UseVideoQuality("", func(q string) error { saved = q; return nil })
	var told []string
	settings.OnVideoQuality(func(q string) { told = append(told, q) })
	var out bytes.Buffer
	if err := videoQuality(&out, settings, nil); err != nil || out.String() != "high\n" {
		t.Fatalf("default = %q, %v", out.String(), err)
	}
	if err := videoQuality(&out, settings, []string{"Saver"}); err != nil || saved != "saver" || settings.VideoQuality() != "saver" {
		t.Fatalf("set: %v, saved %q", err, saved)
	}
	if err := videoQuality(&out, settings, []string{"ultra"}); err == nil || !strings.Contains(err.Error(), "use high, normal or saver") {
		t.Fatalf("bad quality: %v", err)
	}
	if len(told) != 1 || told[0] != "saver" {
		t.Fatalf("listeners told %v", told)
	}
}

func TestParseFlagsAnyOrder(t *testing.T) {
	for _, c := range []struct {
		args  []string
		since time.Duration
		pos   []string
	}{
		{[]string{"#f7d0", "--since", "3h"}, 3 * time.Hour, []string{"#f7d0"}},
		{[]string{"--since", "2h", "room"}, 2 * time.Hour, []string{"room"}},
		{[]string{"a", "--since=1m", "b"}, time.Minute, []string{"a", "b"}},
		{[]string{"a", "--", "--since", "1m"}, 0, []string{"a", "--since", "1m"}},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		since := fs.Duration("since", 0, "")
		if err := parseFlags(fs, c.args); err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if *since != c.since || !slices.Equal(fs.Args(), c.pos) {
			t.Errorf("%v: since %v args %q, want %v %q", c.args, *since, fs.Args(), c.since, c.pos)
		}
	}
}
