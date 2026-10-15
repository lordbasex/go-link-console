// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// release serves a release's files at /<tag>/<name>, SHA256SUMS signed
// with priv.
func release(t *testing.T, priv ed25519.PrivateKey, tag string, files map[string][]byte, tamper func(map[string][]byte)) *httptest.Server {
	t.Helper()
	var sums strings.Builder
	for name, b := range files {
		h := sha256.Sum256(b)
		sums.WriteString(hex.EncodeToString(h[:]) + "  " + name + "\n")
	}
	all := map[string][]byte{"SHA256SUMS": []byte(sums.String())}
	all["SHA256SUMS.sig"] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, all["SHA256SUMS"])) + "\n")
	for name, b := range files {
		all[name] = b
	}
	if tamper != nil {
		tamper(all)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := all[strings.TrimPrefix(r.URL.Path, "/"+tag+"/")]
		if !ok || !strings.HasPrefix(r.URL.Path, "/"+tag+"/") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testUpdater(t *testing.T, base string, pub ed25519.PublicKey) *Updater {
	return &Updater{Base: base, Key: pub, Client: http.DefaultClient, Dir: t.TempDir(), Exe: testExe(t)}
}

func TestDownloadChecksTheSignatureAndTheHash(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	const tag = "v0.4.3"
	u0 := testUpdater(t, "", pub)
	p, err := u0.plan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	name := p.asset(tag)
	body := []byte("the new go-link Console " + strings.Repeat("x", 600<<10))

	t.Run("good", func(t *testing.T) {
		srv := release(t, priv, tag, map[string][]byte{name: body, "other-file": []byte("x")}, nil)
		var last int64
		path, err := testUpdater(t, srv.URL, pub).Download(context.Background(), tag, func(done, _ int64) { last = done })
		if err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != string(body) || last != int64(len(body)) {
			t.Fatalf("got %d bytes, progress %d", len(got), last)
		}
	})
	cases := map[string]func(map[string][]byte){
		"signed by another key": func(all map[string][]byte) {
			all["SHA256SUMS.sig"] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(other, all["SHA256SUMS"])))
		},
		"no signature":      func(all map[string][]byte) { delete(all, "SHA256SUMS.sig") },
		"list changed":      func(all map[string][]byte) { all["SHA256SUMS"] = append(all["SHA256SUMS"], "0  extra\n"...) },
		"file changed":      func(all map[string][]byte) { all[name] = append([]byte("evil"), body...) },
		"file not in list":  func(all map[string][]byte) { all["SHA256SUMS"] = []byte{} },
		"signature garbage": func(all map[string][]byte) { all["SHA256SUMS.sig"] = []byte("not base64!") },
	}
	for what, tamper := range cases {
		t.Run(what, func(t *testing.T) {
			srv := release(t, priv, tag, map[string][]byte{name: body}, tamper)
			u := testUpdater(t, srv.URL, pub)
			path, err := u.Download(context.Background(), tag, nil)
			if !errors.Is(err, ErrSignature) {
				t.Fatalf("err %v, want ErrSignature", err)
			}
			if path != "" {
				t.Fatal("a path for a refused file")
			}
			if left, _ := os.ReadDir(u.Dir); len(left) != 0 {
				t.Fatalf("left %d files behind", len(left))
			}
		})
	}
}

func TestBadTag(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := testUpdater(t, "http://127.0.0.1:1", pub).Download(context.Background(), "../v1", nil); err == nil {
		t.Fatal("a tag with a slash was fetched")
	}
}

// The embedded key is a real Ed25519 public key.
func TestEmbeddedKey(t *testing.T) {
	if len(New().Key) != ed25519.PublicKeySize {
		t.Fatal("the update key does not decode")
	}
}
