// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command appicon writes the macOS app icon set (go-link.iconset) from the
// brand mark drawn by pkg/trayicon; iconutil turns it into go-link.icns.
//
//	go run ./build/macos/appicon DIR/go-link.iconset
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lordbasex/go-link-console/backend-device/pkg/trayicon"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: appicon DIR/go-link.iconset")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The sizes macOS asks for, each at 1x and 2x.
	for _, size := range []int{16, 32, 128, 256, 512} {
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			name := fmt.Sprintf("icon_%dx%d%s.png", size, size, suffix)
			if err := os.WriteFile(filepath.Join(dir, name), trayicon.Render(size*scale, true), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
}
