// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"log/slog"

	"github.com/lordbasex/go-link-console/backend-device/internal/services"
)

// uiOptions is what the user interface needs from the device.
type uiOptions struct {
	ctx      context.Context // canceled when the device stops
	quit     func()          // stops the device
	headless bool
	status   *services.StatusService
	library  *services.LibraryService
	settings *services.SettingsService
	games    *services.GameService // nil when rooms are disabled
	hdGames  *services.HDGamesService
	links    *services.LinkService
	webURL   string // where hosts type the pairing code
	// language is the window's language; setLanguage saves a new one.
	language    string
	setLanguage func(string) error
	logger      *slog.Logger
}
