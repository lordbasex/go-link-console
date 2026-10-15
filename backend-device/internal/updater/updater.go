// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package updater downloads a newer go-link Console and installs it in
// place of this one: the Mac's app bundle, or the folder the Windows .zip
// or the Linux .tar.gz was unpacked in. A file is installed only when the
// release's SHA256SUMS lists its hash and carries a signature by the
// update key (build/signsums), whose public half is below: a release
// someone else put up, or a file changed on its way, is refused.
package updater

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// publicKey checks SHA256SUMS.sig. Its private half is on the release
// computer only (~/.go-link-signing/console-update-ed25519.key).
const publicKey = "U1ywgfH7jCaPl8wQzil/mSXXpJ6Nd97Es+/dFK+rNWY="

// updatePrefix names the temporary folder a download goes in.
const updatePrefix = "go-link-console-update-"

// DefaultBase is where a release's files are, by tag.
const DefaultBase = "https://github.com/lordbasex/go-link-console/releases/download"

// maxAsset bounds a download (the largest file is the Mac's, ~130 MB).
const maxAsset = 300 << 20

// ErrManual: this copy cannot replace itself (a development build, a
// folder it may not write, the window-less build): the person downloads
// the release.
var ErrManual = errors.New("updater: this copy is updated by hand")

// ErrSignature: the release's list of files is not signed by the update
// key, or a file does not match it.
var ErrSignature = errors.New("updater: the download is not go-link Console's")

// Updater downloads and installs releases.
type Updater struct {
	Base   string // DefaultBase, or a test server's
	Key    ed25519.PublicKey
	Client *http.Client
	Dir    string // where downloads go (default: a new temporary folder)
	// Exe is this program (default os.Executable); a test's stand-in.
	Exe string
}

// New is the updater of this program.
func New() *Updater {
	key, _ := base64.StdEncoding.DecodeString(publicKey)
	base := DefaultBase
	// A test server: still only files the update key signed.
	if b := os.Getenv("GOLINK_UPDATE_BASE"); b != "" {
		base = strings.TrimRight(b, "/")
	}
	return &Updater{Base: base, Key: key, Client: &http.Client{Timeout: 10 * time.Minute}}
}

// Progress reports bytes done of a total (0 when not known).
type Progress func(done, total int64)

// Download fetches the release's file for this computer and checks it.
// It returns its path.
func (u *Updater) Download(ctx context.Context, tag string, progress Progress) (string, error) {
	p, err := u.plan()
	if err != nil {
		return "", err
	}
	name := p.asset(tag)
	sums, err := u.get(ctx, tag, "SHA256SUMS", 1<<20, nil)
	if err != nil {
		return "", err
	}
	sig, err := u.get(ctx, tag, "SHA256SUMS.sig", 4<<10, nil)
	if err != nil {
		return "", fmt.Errorf("%w: no signature (%v)", ErrSignature, err)
	}
	want, err := u.verify(sums, sig, name)
	if err != nil {
		return "", err
	}
	dir := u.Dir
	if dir == "" {
		if dir, err = os.MkdirTemp("", updatePrefix); err != nil {
			return "", err
		}
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = u.get(ctx, tag, name, maxAsset, func(r io.Reader, total int64) error {
		var done int64
		buf := make([]byte, 256<<10)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				if _, werr := f.Write(buf[:n]); werr != nil {
					return werr
				}
				h.Write(buf[:n])
				done += int64(n)
				if progress != nil {
					progress(done, total)
				}
			}
			if rerr == io.EOF {
				return nil
			}
			if rerr != nil {
				return rerr
			}
		}
	})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		_ = os.Remove(path)
		return "", fmt.Errorf("%w: %s does not match its hash", ErrSignature, name)
	}
	return path, nil
}

// Install puts the downloaded file in place of this program. On success
// the caller quits; restart opens the new version once it has.
func (u *Updater) Install(ctx context.Context, path string) (restart func() error, err error) {
	p, err := u.plan()
	if err != nil {
		return nil, err
	}
	restart, err = p.install(ctx, path)
	// The download is no longer needed.
	if dir := filepath.Dir(path); strings.HasPrefix(filepath.Base(dir), updatePrefix) {
		_ = os.RemoveAll(dir)
	}
	return restart, err
}

// CanInstall reports whether this copy replaces itself (not ErrManual).
func (u *Updater) CanInstall() bool {
	_, err := u.plan()
	return err == nil
}

// verify checks the list's signature and returns the file's hash in it.
func (u *Updater) verify(sums, sig []byte, name string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(u.Key) != ed25519.PublicKeySize || !ed25519.Verify(u.Key, sums, raw) {
		return "", fmt.Errorf("%w: SHA256SUMS is not signed by the update key", ErrSignature)
	}
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		hash, file, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		file = strings.TrimLeft(strings.TrimSpace(file), "*")
		if ok && file == name && len(hash) == 64 {
			return strings.ToLower(hash), nil
		}
	}
	return "", fmt.Errorf("%w: %s is not in SHA256SUMS", ErrSignature, name)
}

// get fetches a release file; read (when not nil) reads its body.
func (u *Updater) get(ctx context.Context, tag, name string, limit int64, read func(io.Reader, int64) error) ([]byte, error) {
	if strings.ContainsAny(tag, "/?#") {
		return nil, fmt.Errorf("updater: bad tag %q", tag)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.Base+"/"+tag+"/"+name, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: %s: %s", name, resp.Status)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("updater: %s is too large", name)
	}
	body := io.LimitReader(resp.Body, limit+1)
	if read != nil {
		lr := &countReader{r: body}
		if err := read(lr, resp.ContentLength); err != nil {
			return nil, err
		}
		if lr.n > limit {
			return nil, fmt.Errorf("updater: %s is too large", name)
		}
		return nil, nil
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("updater: %s is too large", name)
	}
	return b, nil
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// exe is this program's path, links resolved.
func (u *Updater) exe() (string, error) {
	if u.Exe != "" {
		return u.Exe, nil
	}
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// arch names this computer's architecture as the release files do.
func arch() string { return runtime.GOARCH }
