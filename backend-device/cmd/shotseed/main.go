// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command shotseed fills a throwaway go-link data folder with an invented
// ROM library (made-up games, set names and covers), for the screenshots of
// the landing page (e2e: npm run shots). Never point it at a real folder.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lordbasex/go-link-console/backend-device/internal/shots"
)

func main() {
	data := flag.String("data", "", "the go-link data folder to fill (e.g. <test HOME>/go-link)")
	core := flag.Bool("placeholder-core", true, "leave an empty file where the core goes, so the library reads as ready")
	flag.Parse()
	if *data == "" {
		fmt.Fprintln(os.Stderr, "usage: shotseed -data <folder>")
		os.Exit(2)
	}
	if err := shots.Seed(*data, *core); err != nil {
		fmt.Fprintln(os.Stderr, "shotseed:", err)
		os.Exit(1)
	}
}
