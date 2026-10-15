// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command signsums signs a release's SHA256SUMS with the update key, so
// the app installs only the files it lists: it writes SHA256SUMS.sig
// (an Ed25519 signature, base64) next to it. The private key never
// leaves the release computer (~/.go-link-signing); the public key is in
// internal/updater.
//
//	go run ./build/signsums -gen ~/.go-link-signing/console-update-ed25519.key
//	go run ./build/signsums -key ~/.go-link-signing/console-update-ed25519.key dist/release/v0.4.3/SHA256SUMS
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	gen := flag.String("gen", "", "make a new key in this file and print its public key")
	key := flag.String("key", "", "the private key file")
	flag.Parse()
	if *gen != "" {
		if _, err := os.Stat(*gen); err == nil {
			fail("%s exists: a new key would orphan every app that has the old one", *gen)
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fail("%v", err)
		}
		if err := os.WriteFile(*gen, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			fail("%v", err)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return
	}
	if *key == "" || flag.NArg() != 1 {
		fail("usage: signsums -key KEYFILE SHA256SUMS")
	}
	raw, err := os.ReadFile(*key)
	if err != nil {
		fail("%v", err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		fail("%s is not an update key", *key)
	}
	sums, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fail("%v", err)
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), sums)
	if err := os.WriteFile(flag.Arg(0)+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Println("signed", flag.Arg(0)+".sig")
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "signsums: "+format+"\n", a...)
	os.Exit(1)
}
