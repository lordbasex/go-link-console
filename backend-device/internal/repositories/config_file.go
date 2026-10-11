// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package repositories persists the device configuration.
package repositories

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/ids"
)

// ConfigFile stores models.Config as JSON. The file holds no secrets yet,
// but it is private to the user (0600) because later phases may add some.
type ConfigFile struct {
	path string
}

// DefaultConfigPath returns <user config dir>/go-link-console/device.json,
// for example ~/Library/Application Support/go-link-console/device.json on
// macOS or %AppData%\go-link-console\device.json on Windows.
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "go-link-console", "device.json"), nil
}

// NewConfigFile uses the file at path.
func NewConfigFile(path string) *ConfigFile {
	return &ConfigFile{path: path}
}

// Path returns the file location.
func (f *ConfigFile) Path() string { return f.path }

// newDeviceSecret returns 32 random bytes in unpadded base64url.
func newDeviceSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func validDeviceSecret(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == 32
}

// LoadOrCreate reads the configuration. On first run it creates the file
// with a new permanent device_id and reports created=true.
func (f *ConfigFile) LoadOrCreate() (cfg models.Config, created bool, err error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg = models.Config{DeviceID: ids.NewUUIDv4(), DeviceSecret: newDeviceSecret()}
		if err := f.Save(cfg); err != nil {
			return models.Config{}, false, err
		}
		return cfg, true, nil
	}
	if err != nil {
		return models.Config{}, false, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return models.Config{}, false, fmt.Errorf("parse %s: %w", f.path, err)
	}
	if !ids.ValidUUID(cfg.DeviceID) {
		return models.Config{}, false, fmt.Errorf("parse %s: invalid device_id %q", f.path, cfg.DeviceID)
	}
	// Configurations made before the device secret existed get one now.
	if !validDeviceSecret(cfg.DeviceSecret) {
		cfg.DeviceSecret = newDeviceSecret()
		if err := f.Save(cfg); err != nil {
			return models.Config{}, false, err
		}
	}
	// Tighten permissions if someone loosened them.
	if info, err := os.Stat(f.path); err == nil && info.Mode().Perm() != 0o600 {
		_ = os.Chmod(f.path, 0o600)
	}
	return cfg, false, nil
}

// Save writes the configuration atomically: a temporary file is written
// and renamed over the old one, so a crash never leaves half a file.
func (f *ConfigFile) Save(cfg models.Config) error {
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".device-*.json")
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("save config: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("save config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
