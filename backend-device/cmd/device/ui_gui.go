// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package main

import "github.com/lordbasex/go-link/backend-device/internal/gui"

// runUI shows the native window and tray icon, unless the device runs
// headless. It blocks until the device stops.
func runUI(o uiOptions) {
	if o.headless {
		<-o.ctx.Done()
		return
	}
	gui.Run(gui.Options{
		Ctx:         o.ctx,
		Quit:        o.quit,
		Status:      o.status,
		Library:     o.library,
		Games:       o.games,
		HDGames:     o.hdGames,
		Links:       o.links,
		Settings:    o.settings,
		WebURL:      o.webURL,
		Language:    o.language,
		SetLanguage: o.setLanguage,
		Version:     version,
		Logger:      o.logger,
	})
}

// guiBuilt: this build has the window; --headless still turns it off.
const guiBuilt = true
