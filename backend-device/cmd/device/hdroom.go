// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"fmt"
	"slices"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
)

// hdRoomVideo picks go-link HD game rooms' video: the H.264 encoder ("" for
// VP8) and the bitrate at high quality. codec is --hd-room-codec: vp8, h264
// (with the encoder h264, --hd-h264: videotoolbox, the Mac's hardware called
// directly, or x264 through ffmpeg) or auto, the Mac's hardware on macOS and
// VP8 like every room elsewhere. kbps 0
// is by the rooms' size: 4000 at 720p (scale 2), 8000 at 1080p (scale 3).
func hdRoomVideo(codec, h264 string, kbps, scale int, goos string, ffmpeg func() (string, error)) (string, int, error) {
	if kbps <= 0 {
		kbps = 4000
		if scale >= 3 {
			kbps = 8000
		}
	}
	switch codec {
	case "vp8":
		return "", kbps, nil
	case "h264":
		if !slices.Contains(encoder.H264Encoders, h264) {
			return "", 0, fmt.Errorf("--hd-h264: %q is not x264 or videotoolbox", h264)
		}
		if h264 == "videotoolbox" && goos != "darwin" {
			return "", 0, fmt.Errorf("--hd-h264 videotoolbox: only on macOS")
		}
		if h264 == "x264" {
			if _, err := ffmpeg(); err != nil {
				return "", 0, fmt.Errorf("--hd-room-codec h264: %w", err)
			}
		}
		return h264, kbps, nil
	case "auto":
		if goos == "darwin" {
			return "videotoolbox", kbps, nil
		}
		return "", kbps, nil
	}
	return "", 0, fmt.Errorf("--hd-room-codec: %q is not vp8, h264 or auto", codec)
}
