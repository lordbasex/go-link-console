// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows || !cgo

package encoder

import "errors"

// Media Foundation is Windows' (with cgo): elsewhere the encoder "mediafoundation" is not there.
type mfEncoder struct {
	name     string
	hardware bool
}

func newMF(cfg Config, gop int, hardware bool, onFrame func([]byte)) (*mfEncoder, error) {
	return nil, errors.New("h264: Media Foundation is only on Windows")
}

func (m *mfEncoder) write(i420 []byte, key bool) error {
	return errors.New("h264: no Media Foundation")
}
func (m *mfEncoder) close() {}

// MFHardware is "" off Windows.
func MFHardware() string { return "" }

const hasMF = false
