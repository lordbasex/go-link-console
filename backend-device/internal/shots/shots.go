// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package shots fills a go-link data folder with an invented ROM library for
// the website's and the window's screenshots (the landing page gallery).
// Every game, set name, maker and cover here is made up: no real game, ROM
// set, company or artwork appears in a screenshot. The zips hold a few
// bytes of filler, the game list describes exactly those bytes, and the
// covers are drawn in code (a mirrored pixel sprite over a gradient).
package shots

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/lordbasex/go-link-console/backend-device/pkg/cores"
	"github.com/lordbasex/go-link-console/backend-device/pkg/romcheck"
)

// Game is one invented game of the library.
type Game struct {
	Set, Title, Year, Maker string
	Players, Buttons        int
	// Missing leaves one required file out of the zip, so the library
	// shows a set that will not run.
	Missing bool
	// Top and Bottom are the cover's gradient colors, Ink its sprite color.
	Top, Bottom, Ink color.RGBA
}

// Games is the invented library, in a fixed order.
var Games = []Game{
	{Set: "skypir99", Title: "Sky Pirates 2099", Year: "1992", Maker: "Tin Owl Works", Players: 2, Buttons: 3, Top: rgb(0x1b, 0x2a, 0x6b), Bottom: rgb(0x6a, 0x2b, 0xd0), Ink: rgb(0xff, 0xc0, 0x6a)},
	{Set: "cometcus", Title: "Comet Custodians", Year: "1987", Maker: "Quasar Pond", Players: 2, Buttons: 2, Top: rgb(0x08, 0x10, 0x28), Bottom: rgb(0x1f, 0x6f, 0x8b), Ink: rgb(0x9f, 0xf5, 0xe0)},
	{Set: "pklheist", Title: "Pickle Harbor Heist", Year: "1990", Maker: "Tin Owl Works", Players: 4, Buttons: 2, Top: rgb(0x14, 0x3d, 0x1e), Bottom: rgb(0x7c, 0xb3, 0x42), Ink: rgb(0xff, 0xf1, 0x76)},
	{Set: "robolndr", Title: "Robo Laundry Blitz", Year: "1994", Maker: "Quasar Pond", Players: 2, Buttons: 3, Top: rgb(0x2b, 0x0f, 0x3a), Bottom: rgb(0xd8, 0x4a, 0x8a), Ink: rgb(0xa8, 0xe6, 0xff)},
	{Set: "glacgoal", Title: "Glacier Goalies", Year: "1989", Maker: "Tin Owl Works", Players: 4, Buttons: 2, Top: rgb(0x0d, 0x2c, 0x4a), Bottom: rgb(0x8e, 0xc9, 0xf0), Ink: rgb(0xff, 0xff, 0xff)},
	{Set: "vthunder", Title: "Velvet Thunder Bowl", Year: "1995", Maker: "Quasar Pond", Players: 2, Buttons: 4, Top: rgb(0x3a, 0x0a, 0x12), Bottom: rgb(0xb8, 0x32, 0x3c), Ink: rgb(0xff, 0xd7, 0x8a)},
	{Set: "lmothexp", Title: "Lantern Moth Express", Year: "1991", Maker: "Tin Owl Works", Players: 2, Buttons: 2, Top: rgb(0x1a, 0x14, 0x05), Bottom: rgb(0xc2, 0x7c, 0x0e), Ink: rgb(0xff, 0xf6, 0xc8)},
	{Set: "cactusrc", Title: "Cactus Circuit Racers", Year: "1993", Maker: "Quasar Pond", Players: 4, Buttons: 2, Missing: true, Top: rgb(0x3b, 0x1c, 0x0a), Bottom: rgb(0xe0, 0x8a, 0x3a), Ink: rgb(0x8f, 0xe0, 0x8a)},
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 0xff} }

// Seed writes the invented library into base (a go-link data folder, like
// ~/go-link): the zips in roms/, the game list in cores/ and a Boxart per
// game in thumbnails/MAME. With placeholderCore it also leaves an empty file
// where the emulator core goes, so the library reads as ready; that file
// is not a core and must never be started.
func Seed(base string, placeholderCore bool) error {
	roms := filepath.Join(base, "roms")
	coresDir := filepath.Join(base, "cores")
	boxarts := filepath.Join(base, "thumbnails", "MAME", "Named_Boxarts")
	for _, d := range []string{roms, coresDir, boxarts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	var xml strings.Builder
	xml.WriteString("<mame>\n")
	for _, g := range Games {
		files := [][2]string{{g.Set + ".p1", "program " + g.Set}, {g.Set + ".gfx", "tiles " + g.Set}}
		fmt.Fprintf(&xml, "<game name=%q><description>%s</description><year>%s</year><manufacturer>%s</manufacturer>", g.Set, g.Title, g.Year, g.Maker)
		for _, f := range files {
			fmt.Fprintf(&xml, "<rom name=%q size=\"%d\" crc=\"%08x\"/>", f[0], len(f[1]), crc32.ChecksumIEEE([]byte(f[1])))
		}
		fmt.Fprintf(&xml, "<driver status=\"good\"/><input players=\"%d\" buttons=\"%d\" control=\"joy8way\"/></game>\n", g.Players, g.Buttons)
		if g.Missing {
			files = files[:1]
		}
		if err := writeZip(filepath.Join(roms, g.Set+".zip"), files, 900_000+len(g.Title)*97_000); err != nil {
			return err
		}
		if err := writeCover(filepath.Join(boxarts, g.Set+".png"), g); err != nil {
			return err
		}
	}
	xml.WriteString("</mame>\n")
	cat, err := romcheck.ParseXML(strings.NewReader(xml.String()))
	if err != nil {
		return err
	}
	if err := cat.Save(filepath.Join(coresDir, romcheck.FileName)); err != nil {
		return err
	}
	if placeholderCore {
		return os.WriteFile(filepath.Join(coresDir, cores.FileName(cores.DefaultCore, runtime.GOOS)), nil, 0o644)
	}
	return nil
}

// writeZip stores the files plus filler bytes (not a required file), so
// the folder size looks like a real library's.
func writeZip(path string, files [][2]string, filler int) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	pad := make([]byte, filler)
	x := uint32(filler)
	for i := range pad {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		pad[i] = byte(x)
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "filler.bin", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := w.Write(pad); err != nil {
		return err
	}
	for _, f := range files {
		w, err := zw.Create(f[0])
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Cover draws a game's invented box art, simple and geometric: a gradient,
// a big turned shape, a small disc and a bar, and the title in a pixel font.
// Nothing is traced from real art.
func Cover(g Game) *image.RGBA {
	const w, h = 300, 400
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		c := mix(g.Top, g.Bottom, float64(y)/float64(h-1))
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	seed := crc32.ChecksumIEEE([]byte(g.Set))
	shape := int(seed % 3)          // 0 square, 1 disc, 2 rounded diamond
	turn := float64(seed%60) - 30.0 // degrees
	accent := mix(g.Ink, g.Top, 0.35)
	// The big shape, centered a little above the middle.
	cx, cy, r := 150.0, 190.0, 78.0
	sin, cos := math.Sincos(turn * math.Pi / 180)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			u, v := dx*cos+dy*sin, -dx*sin+dy*cos
			in := false
			switch shape {
			case 0:
				in = math.Abs(u) <= r && math.Abs(v) <= r
			case 1:
				in = u*u+v*v <= r*r*1.1
			default:
				in = math.Abs(u)+math.Abs(v) <= r*1.35 && math.Max(math.Abs(u), math.Abs(v)) <= r*0.95
			}
			if in {
				img.SetRGBA(x, y, g.Ink)
			}
		}
	}
	// A disc and a bar at the top left.
	for y := 30; y < 70; y++ {
		for x := 30; x < 70; x++ {
			if (x-50)*(x-50)+(y-50)*(y-50) <= 400 {
				img.SetRGBA(x, y, accent)
			}
		}
	}
	fill(img, image.Rect(30, 84, 118, 94), accent)
	// The title under the shape, wrapped on words, in a 7x13 font scaled 2x.
	lines := wrap(strings.ToUpper(g.Title), 16)
	top := h - 30 - len(lines)*30
	for i, line := range lines {
		pixelText(img, line, 2, w/2, top+i*30+2, color.RGBA{0, 0, 0, 140}, 2)
		pixelText(img, line, 2, w/2, top+i*30, color.RGBA{255, 255, 255, 255}, 0)
	}
	return img
}

func writeCover(path string, g Game) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, Cover(g)); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 0xff}
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Over)
}

// wrap splits words into lines of at most n characters.
func wrap(s string, n int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len(line)+1+len(word) > n {
			lines = append(lines, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	return append(lines, line)
}

// pixelText draws s centered on cx with its top at y, each font pixel as a
// scale x scale block (shift moves a shadow copy right).
func pixelText(dst *image.RGBA, s string, scale, cx, y int, c color.RGBA, shift int) {
	face := basicfont.Face7x13
	adv := font.MeasureString(face, s).Ceil()
	small := image.NewAlpha(image.Rect(0, 0, adv, 13))
	d := font.Drawer{Dst: small, Src: image.Opaque, Face: face, Dot: fixed.P(0, 10)}
	d.DrawString(s)
	x0 := cx - adv*scale/2 + shift
	for py := 0; py < 13; py++ {
		for px := 0; px < adv; px++ {
			if small.AlphaAt(px, py).A < 128 {
				continue
			}
			fill(dst, image.Rect(x0+px*scale, y+py*scale, x0+(px+1)*scale, y+(py+1)*scale), c)
		}
	}
}
