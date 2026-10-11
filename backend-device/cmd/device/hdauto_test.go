// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"reflect"
	"testing"

	"github.com/lordbasex/go-link-console/backend-device/pkg/encoder"
)

func TestHDCandidatesFollowTheExperiment(t *testing.T) {
	sizes := func(c []hdChoice) []string {
		var out []string
		for _, x := range c {
			out = append(out, x.Size+" "+x.Codec+" "+x.H264)
		}
		return out
	}
	all := encoder.H264Support{FFmpeg: "ffmpeg", X264: true, VideoToolbox: true}
	// Apple Silicon: 4K with x264 first (4K60 from the M1), then the hardware encoder's 1080p
	if got, want := sizes(hdCandidates(all, 8, true)), []string{"2160p h264 x264", "1080p h264 videotoolbox", "1080p h264 x264", "1080p vp8 ", "720p vp8 "}; !reflect.DeepEqual(got, want) {
		t.Errorf("Apple Silicon: %v, want %v", got, want)
	}
	// an Intel Mac: no 4K
	if got := sizes(hdCandidates(all, 20, false)); got[0] != "1080p h264 videotoolbox" {
		t.Errorf("Intel Mac starts with %v", got)
	}
	// no ffmpeg and few cores: only 720p VP8
	if got, want := sizes(hdCandidates(encoder.H264Support{}, 4, false)), []string{"720p vp8 "}; !reflect.DeepEqual(got, want) {
		t.Errorf("no ffmpeg: %v, want %v", got, want)
	}
}

func TestHDFitsUsesEachEncodersRule(t *testing.T) {
	if ok, _ := hdFits(hdResult{Realtime: true}, "vp8"); !ok {
		t.Error("VP8 within 60 % of a frame should fit")
	}
	if ok, why := hdFits(hdResult{Realtime: false, EncodeP95Ms: 14}, "vp8"); ok || why == "" {
		t.Error("VP8 over budget should not fit, with a reason")
	}
	if ok, _ := hdFits(hdResult{MaxFPS: 110}, "h264"); !ok {
		t.Error("x264 at 110 fps should fit")
	}
	if ok, _ := hdFits(hdResult{MaxFPS: 54}, "h264"); ok {
		t.Error("54 fps should not fit")
	}
}
