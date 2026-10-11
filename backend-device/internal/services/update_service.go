// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

// DefaultReleasesURL lists go-link's releases, newest first.
const DefaultReleasesURL = "https://api.github.com/repos/lordbasex/go-link-console/releases?per_page=10"

// UpdateConfig configures UpdateService.
type UpdateConfig struct {
	Current  string        // this build's version, e.g. v0.1.0
	URL      string        // releases list (GitHub API); empty = DefaultReleasesURL
	Every    time.Duration // time between checks; 0 = 6 hours
	FirstIn  time.Duration // delay of the first check; 0 = 30 seconds
	Client   *http.Client
	Logger   *slog.Logger
	OnUpdate func(models.UpdateInfo) // called when a newer release shows up
}

// UpdateService checks now and then whether a newer go-link was released,
// so the window and the linked browsers can offer the download. It only
// reads the public releases list; nothing is downloaded or installed.
type UpdateService struct{ cfg UpdateConfig }

// NewUpdateService makes the checker; Run starts it.
func NewUpdateService(cfg UpdateConfig) *UpdateService {
	if cfg.URL == "" {
		cfg.URL = DefaultReleasesURL
	}
	if cfg.Every <= 0 {
		cfg.Every = 6 * time.Hour
	}
	if cfg.FirstIn <= 0 {
		cfg.FirstIn = 30 * time.Second
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &UpdateService{cfg: cfg}
}

// Run checks until ctx ends. A build without a release version (a
// development build) is never told to update.
func (u *UpdateService) Run(ctx context.Context) {
	if _, ok := parseVersion(u.cfg.Current); !ok {
		return
	}
	wait := u.cfg.FirstIn
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = u.cfg.Every
		info, err := u.Check(ctx)
		if err != nil {
			u.cfg.Logger.Debug("cannot check for updates", "err", err)
			continue
		}
		if info.Latest != "" && u.cfg.OnUpdate != nil {
			u.cfg.Logger.Info("a new go-link version is available", "current", u.cfg.Current, "latest", info.Latest)
			u.cfg.OnUpdate(info)
		}
	}
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
}

// Check asks for the releases and returns the newest one when it is newer
// than this build (an empty Latest otherwise).
func (u *UpdateService) Check(ctx context.Context) (models.UpdateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.URL, nil)
	if err != nil {
		return models.UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "go-link-console/"+u.cfg.Current)
	resp, err := u.cfg.Client.Do(req)
	if err != nil {
		return models.UpdateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.UpdateInfo{}, fmt.Errorf("releases: %s", resp.Status)
	}
	var list []release
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&list); err != nil {
		return models.UpdateInfo{}, err
	}
	current, _ := parseVersion(u.cfg.Current)
	var best release
	var bestV [3]int
	for _, r := range list {
		v, ok := parseVersion(r.TagName)
		if r.Draft || !ok {
			continue
		}
		if best.TagName == "" || newer(v, bestV) {
			best, bestV = r, v
		}
	}
	if best.TagName == "" || !newer(bestV, current) {
		return models.UpdateInfo{}, nil
	}
	return models.UpdateInfo{Latest: best.TagName, URL: best.HTMLURL}, nil
}

// parseVersion reads vX.Y.Z (with or without the v); a suffix such as
// -dev or -rc1 is ignored. Other strings (a commit hash) are not versions.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func newer(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
