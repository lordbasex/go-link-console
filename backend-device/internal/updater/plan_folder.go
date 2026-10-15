// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// oldSuffix marks a file an update replaced (Cleanup removes them).
const oldSuffix = ".old"

// folderPlan updates the folder the Windows .zip or the Linux .tar.gz was
// unpacked in: the release's archive is unpacked next to this program and
// each of its files takes the place of the one with its name. The files
// in use are renamed first (Windows lets a running program and its
// libraries be renamed, not overwritten) and removed once the new version
// starts. Settings, games and saves live elsewhere and are not touched.
type folderPlan struct {
	dir     string // the folder with the program
	exe     string // this program
	lib     string // go-link HD's engine next to it, which the archive must bring too
	pattern string // the release file, with %s for the tag
	restart func(exe string) func() error
}

// folderOf is the plan of a program unpacked from an archive: name and lib
// are the files a release's folder has; nil when exe is not such a copy.
func folderOf(exe, name, lib, asset string, restart func(string) func() error) (plan, error) {
	dir := filepath.Dir(exe)
	if !strings.EqualFold(filepath.Base(exe), name) {
		return nil, ErrManual // renamed, or a development build
	}
	if _, err := os.Stat(filepath.Join(dir, lib)); err != nil {
		return nil, ErrManual
	}
	if !writable(dir) {
		return nil, ErrManual
	}
	return &folderPlan{dir: dir, exe: exe, lib: lib, pattern: asset, restart: restart}, nil
}

func (p *folderPlan) asset(tag string) string { return fmt.Sprintf(p.pattern, tag) }

func (p *folderPlan) install(ctx context.Context, archive string) (func() error, error) {
	stage, err := os.MkdirTemp(p.dir, ".go-link-console-new-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	if strings.HasSuffix(archive, ".zip") {
		err = unzip(archive, stage)
	} else {
		err = untar(archive, stage)
	}
	if err != nil {
		return nil, err
	}
	for _, need := range []string{filepath.Base(p.exe), p.lib} {
		if _, err := os.Stat(filepath.Join(stage, need)); err != nil {
			return nil, fmt.Errorf("updater: the archive has no %s", need)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	// Each file in turn; the first that fails puts back the ones before it.
	var done []string
	undo := func() {
		for i := len(done) - 1; i >= 0; i-- {
			target := filepath.Join(p.dir, done[i])
			_ = os.Remove(target)
			_ = os.Rename(target+oldSuffix, target)
		}
	}
	for _, name := range names {
		target := filepath.Join(p.dir, name)
		_ = os.Remove(target + oldSuffix)
		if _, err := os.Lstat(target); err == nil {
			if err := os.Rename(target, target+oldSuffix); err != nil {
				undo()
				return nil, err
			}
		}
		if err := os.Rename(filepath.Join(stage, name), target); err != nil {
			_ = os.Rename(target+oldSuffix, target)
			undo()
			return nil, err
		}
		done = append(done, name)
	}
	// What is not in use any more goes now (on Windows, the next start).
	for _, name := range done {
		_ = os.Remove(filepath.Join(p.dir, name) + oldSuffix)
	}
	return p.restart(p.exe), nil
}

// flatName is an archive entry's file name, or an error for anything but
// a plain file at the archive's top (a release's archives are flat).
func flatName(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.HasSuffix(name, oldSuffix) {
		return "", fmt.Errorf("updater: %q is not a file of a release", name)
	}
	return name, nil
}

func writeFile(path string, r io.Reader, exec bool) error {
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(r, maxAsset+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxAsset {
		err = errors.New("updater: a file in the archive is too large")
	}
	return err
}

// untar unpacks a flat .tar.gz into dir.
func untar(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("updater: %q is not a file of a release", h.Name)
		}
		name, err := flatName(strings.TrimPrefix(h.Name, "./"))
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, name), tr, h.Mode&0o111 != 0); err != nil {
			return err
		}
	}
}

// unzip unpacks a flat .zip into dir.
func unzip(archive, dir string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			return fmt.Errorf("updater: %q is not a file of a release", f.Name)
		}
		name, err := flatName(f.Name)
		if err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(filepath.Join(dir, name), r, strings.HasSuffix(name, ".exe"))
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
