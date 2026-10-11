// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package cores downloads libretro emulator cores from the official
// libretro buildbot. Cores are never bundled with the device: the host
// downloads the one it needs, for its own operating system.
package cores

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultCore is the MAME core the device uses.
const DefaultCore = "mame2003_plus"

// BuildbotURL is the root of libretro's nightly builds.
const BuildbotURL = "https://buildbot.libretro.com/nightly"

// maxZip bounds the download (mame2003-plus is ~10 MB zipped).
const maxZip = 128 << 20

// FileName returns the library name of a core for an OS, e.g.
// mame2003_plus_libretro.dylib.
func FileName(core, goos string) string {
	ext := ".so"
	switch goos {
	case "darwin":
		ext = ".dylib"
	case "windows":
		ext = ".dll"
	}
	return core + "_libretro" + ext
}

// URL returns where the buildbot publishes a core for an OS and CPU.
func URL(base, core, goos, goarch string) (string, error) {
	var platform string
	switch {
	case goos == "darwin" && goarch == "amd64":
		platform = "apple/osx/x86_64"
	case goos == "darwin" && goarch == "arm64":
		platform = "apple/osx/arm64"
	case goos == "linux" && goarch == "amd64":
		platform = "linux/x86_64"
	case goos == "linux" && goarch == "arm64":
		platform = "linux/aarch64" // Raspberry Pi OS 64-bit
	case goos == "linux" && goarch == "arm":
		platform = "linux/armhf" // Raspberry Pi OS 32-bit
	case goos == "windows" && goarch == "amd64":
		platform = "windows/x86_64"
	default:
		return "", fmt.Errorf("cores: no prebuilt core for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("%s/%s/latest/%s.zip", base, platform, FileName(core, goos)), nil
}

// ErrCoreChanged means the core on disk is not the one that was
// downloaded: something modified it, so it is not loaded.
var ErrCoreChanged = errors.New("the emulator core changed on disk since it was downloaded: download it again")

func sumPath(core string) string { return core + ".sha256" }

func writeSum(core string, data []byte) error {
	sum := sha256.Sum256(data)
	return os.WriteFile(sumPath(core), []byte(hex.EncodeToString(sum[:])+"\n"), 0o600)
}

// Verify checks that the core at path is still the file that was
// downloaded, before the device runs its code. A core installed before
// hashes were kept gets its hash now.
func Verify(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	want, err := os.ReadFile(sumPath(path))
	if errors.Is(err, fs.ErrNotExist) {
		return writeSum(path, data)
	}
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if strings.TrimSpace(string(want)) != hex.EncodeToString(sum[:]) {
		return ErrCoreChanged
	}
	return nil
}

// Download fetches a core zip and installs the library in dir. It returns
// the installed path.
func Download(ctx context.Context, client *http.Client, url, fileName, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "go-link-console")
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cores: %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxZip+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxZip {
		return "", fmt.Errorf("cores: download larger than %d bytes", maxZip)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("cores: not a zip: %w", err)
	}
	for _, f := range zr.File {
		// Only the expected library, by its base name: no paths from
		// the archive are ever used.
		if filepath.Base(f.Name) != fileName || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxZip*4))
		rc.Close()
		if err != nil {
			return "", err
		}
		// Only the user may write the cores: the device runs their code.
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		tmp, err := os.CreateTemp(dir, ".core-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			return "", err
		}
		if err := tmp.Chmod(0o700); err != nil {
			tmp.Close()
			return "", err
		}
		if err := tmp.Close(); err != nil {
			return "", err
		}
		path := filepath.Join(dir, fileName)
		if err := os.Rename(tmp.Name(), path); err != nil {
			return "", err
		}
		// Remember what was installed, so a later change is noticed
		// before the core is loaded (Verify).
		if err := writeSum(path, data); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("cores: %s not found in the archive", fileName)
}
