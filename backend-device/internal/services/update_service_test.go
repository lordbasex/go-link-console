// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
)

func TestUpdateServiceFindsANewerRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"tag_name":"v0.3.0","html_url":"https://example.com/v0.3.0","draft":true},
			{"tag_name":"v0.2.9","html_url":"https://example.com/v0.2.9","prerelease":true},
			{"tag_name":"v0.2.1","html_url":"https://example.com/v0.2.1","draft":false},
			{"tag_name":"v0.1.0","html_url":"https://example.com/v0.1.0","draft":false},
			{"tag_name":"nightly","html_url":"https://example.com/nightly","draft":false}
		]`))
	}))
	defer srv.Close()
	check := func(current string) string {
		info, err := NewUpdateService(UpdateConfig{Current: current, URL: srv.URL}).Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return info.Latest
	}
	if got := check("v0.1.0"); got != "v0.2.1" {
		t.Fatalf("from v0.1.0: %q (drafts, pre-releases and non-versions are skipped)", got)
	}
	if got := check("v0.2.1"); got != "" {
		t.Fatalf("up to date: %q", got)
	}
	if got := check("v0.3.0-dev"); got != "" {
		t.Fatalf("a newer build: %q", got)
	}
	if _, ok := parseVersion("da1aa4e"); ok {
		t.Fatal("a commit hash is not a version")
	}
}

func TestTheUpdateReachesLinkedBrowsers(t *testing.T) {
	st := models.Status{DeviceID: "d", Version: "v0.1.0", Update: &models.UpdateInfo{Latest: "v0.1.1", URL: "https://github.com/lordbasex/go-link-console/releases/tag/v0.1.1"}}
	b, err := json.Marshal(NewDeviceStatusMessage(st))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"update":{"latest":"v0.1.1","url":"https://github.com/lordbasex/go-link-console/releases/tag/v0.1.1"}`) {
		t.Fatalf("device_status = %s", b)
	}
}
