// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package glhd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Demo is a game built into go-link HD's engine: it needs no package, so
// every device with the engine can play it.
type Demo struct {
	Name    string // its name in the library, like a ROM set's
	Index   int    // golinkhd_load_demo's number
	Title   string
	Players int
}

// Demos are the engine's built-in games, listed in the library whenever
// the engine is installed.
var Demos = []Demo{
	{Name: "glhd_platformer", Index: 0, Title: "go-link HD: Platformer", Players: 4},
	{Name: "glhd_showcase", Index: 1, Title: "go-link HD: Showcase", Players: 4},
}

// FindDemo returns the built-in game of a library name.
func FindDemo(name string) (Demo, bool) {
	for _, d := range Demos {
		if d.Name == name {
			return d, true
		}
	}
	return Demo{}, false
}

// demoScheme marks a built-in game where a package path goes (the worker's
// --rom), so the engine starts it instead of reading a file.
const demoScheme = "golinkhd-demo:"

// DemoPath is the "path" of a built-in game.
func DemoPath(d Demo) string { return demoScheme + strconv.Itoa(d.Index) }

// DemoIndex tells whether a path is a built-in game's, and which.
func DemoIndex(path string) (int, bool) {
	s, ok := strings.CutPrefix(path, demoScheme)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// BundledLibrary is where a release ships the engine library next to the
// device's executable: inside go-link-console.app's Frameworks folder on macOS,
// beside the program elsewhere. It returns "" when there is none.
func BundledLibrary(goos string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	dir := filepath.Dir(exe)
	candidates := []string{filepath.Join(dir, LibraryFile(goos))}
	if goos == "darwin" {
		candidates = append([]string{filepath.Join(dir, "..", "Frameworks", LibraryFile(goos))}, candidates...)
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return filepath.Clean(p)
		}
	}
	return ""
}
