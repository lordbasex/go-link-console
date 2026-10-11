// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

func TestLoaderLine(t *testing.T) {
	for _, c := range []struct{ msg, name, verdict string }{
		{"[MAME 2003+] mbe_23e.rom  WRONG CHECKSUMS:", "mbe_23e.rom", "wrong_checksum"},
		{"[MAME 2003+] MB_05.BIN    NOT FOUND", "mb_05.bin", "not_found"},
		{"[MAME 2003+] mb_06.bin    WRONG LENGTH (expected: 00080000 found: 00040000)", "mb_06.bin", "bad_length"},
		{"[MAME 2003+] hiscore.dat NOT FOUND", "", ""},
		{"[MAME 2003+]     EXPECTED: CRC(5394057a) SHA1(57f8b40c)", "", ""},
		{"[MAME 2003+] Opening ROM file: mbe_24b.rom", "", ""},
	} {
		name, verdict := loaderLine(c.msg)
		if name != c.name || verdict != c.verdict {
			t.Errorf("%q: %q %q", c.msg, name, verdict)
		}
	}
}

// romTestWorker runs this test binary as the "romtest --child" worker.
func romTestWorker(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestRomTestHelper$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_ROMTEST_WORKER=1")
	return cmd
}

// TestRomTestHelper is the worker process of TestRomTestWithRealCore.
func TestRomTestHelper(t *testing.T) {
	if os.Getenv("GO_WANT_ROMTEST_WORKER") != "1" {
		t.Skip("helper process")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+2:] // drop "romtest"
			break
		}
	}
	if err := runRomTestChild(args); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestRomTestWithRealCore powers on go-link's own set (rom/build, made by
// rom/tools/build.mjs) with the real core, then a copy whose program is
// erased. It needs the mame2003-plus core in ~/go-link-console and the built set,
// and is skipped otherwise (CI has neither).
func TestRomTestWithRealCore(t *testing.T) {
	home, _ := os.UserHomeDir()
	coresDir := filepath.Join(home, "go-link-console", "cores")
	corePath := filepath.Join(coresDir, cores.FileName(cores.DefaultCore, runtime.GOOS))
	built := filepath.Join("..", "..", "..", "rom", "build", "slammast.zip")
	if _, err := os.Stat(corePath); err != nil {
		t.Skip("mame2003-plus core not installed")
	}
	if _, err := os.Stat(built); err != nil {
		t.Skip("rom/build/slammast.zip not built")
	}
	cat, _ := romcheck.Load(filepath.Join(coresDir, romcheck.FileName))
	tests := services.NewRomTestService(services.RomTestConfig{
		Dir:      filepath.Join(t.TempDir(), "romtest"),
		CorePath: func() string { return corePath },
		Catalog:  func() *romcheck.Catalog { return cat },
		Command:  romTestWorker,
	})
	res := tests.RunFile(context.Background(), built, 0)
	// time.realtime may fail here: -race makes the core several times slower.
	for _, s := range res.Steps {
		if !s.OK && s.Name != "time.realtime" {
			t.Fatalf("our set failed: %+v", res.Steps)
		}
	}
	if len(res.Steps) != 10 || res.Shot == "" || res.Own == nil || res.Error != "" {
		t.Fatalf("our set: %+v", res)
	}

	// The same set with its program erased: it must fail, with a reason.
	bad := filepath.Join(t.TempDir(), "slammast.zip")
	eraseProgram(t, built, bad, "mbe_23e.rom")
	res = tests.RunFile(context.Background(), bad, 0)
	if res.OK || res.Own != nil {
		t.Fatalf("an erased program passed: %+v", res.Steps)
	}
	failed := false
	for _, s := range res.Steps {
		if !s.OK && s.Detail != "" && s.Name != "time.realtime" {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("no failing step explains it: %+v", res.Steps)
	}
}

// eraseProgram copies a zip with one file's bytes set to 0xff.
func eraseProgram(t *testing.T, src, dst, name string) {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == name {
			data = bytes.Repeat([]byte{0xff}, len(data))
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
