// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"time"

	"github.com/lordbasex/go-link-console/backend-device/internal/models"
	"github.com/lordbasex/go-link-console/backend-device/pkg/sysinfo"
)

// DeviceStatusMessage is sent to linked browsers on the "control"
// DataChannel. It never includes the pairing code.
type DeviceStatusMessage struct {
	Type      string               `json:"type"` // "device_status"
	DeviceID  string               `json:"device_id"`
	Version   string               `json:"version"`
	SignalURL string               `json:"signal_url"`
	ICEURLs   []string             `json:"ice_urls"`
	RomsDir   string               `json:"roms_dir"`
	System    *models.SystemStatus `json:"system,omitempty"`
	Room      *models.RoomStatus   `json:"room,omitempty"`
	Library   *models.Library      `json:"library,omitempty"`
	Linked    int                  `json:"linked_browsers"`
	Rooms     []models.ManagedRoom `json:"rooms"`
	// SavesBytes is the space the rooms' saved games take.
	SavesBytes int64 `json:"saves_bytes"`
	// Update is a newer go-link release, when there is one.
	Update *models.UpdateInfo `json:"update,omitempty"`
	// VideoQuality is the host's video quality for game rooms.
	VideoQuality string `json:"video_quality"`
}

// NewDeviceStatusMessage builds the message from a status snapshot.
func NewDeviceStatusMessage(st models.Status) DeviceStatusMessage {
	return DeviceStatusMessage{
		Type:       "device_status",
		DeviceID:   st.DeviceID,
		Version:    st.Version,
		SignalURL:  st.Signal.URL,
		ICEURLs:    st.Signal.ICEURLs,
		RomsDir:    st.RomsDir,
		System:     st.System,
		Room:       st.Room,
		Library:    st.Library,
		Linked:     len(st.Peers),
		Rooms:      st.Rooms,
		SavesBytes: st.SavesBytes,
		Update:     st.Update,

		VideoQuality: st.VideoQuality,
	}
}

// MetricsService samples CPU and RAM and stores them in the status, which
// the local panel (and a future native GUI) reads. After every sample it
// calls publish, which forwards the status to linked browsers.
type MetricsService struct {
	status   *StatusService
	every    time.Duration
	publish  func(models.Status)
	sampler  func(context.Context) sysinfo.Usage
	hardware sysinfo.Hardware
}

// NewMetricsService reads the hardware once and prepares the sampler.
func NewMetricsService(ctx context.Context, status *StatusService, every time.Duration, publish func(models.Status)) *MetricsService {
	if every <= 0 {
		every = 2 * time.Second
	}
	s := sysinfo.NewSampler()
	return &MetricsService{status: status, every: every, publish: publish, sampler: s.Sample, hardware: sysinfo.ReadHardware(ctx)}
}

// Run samples until ctx ends.
func (m *MetricsService) Run(ctx context.Context) {
	t := time.NewTicker(m.every)
	defer t.Stop()
	for {
		m.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *MetricsService) tick(ctx context.Context) {
	m.status.SetSystem(models.SystemStatus{Hardware: m.hardware, Usage: m.sampler(ctx), SampledAt: time.Now()})
	if m.publish != nil {
		m.publish(m.status.Snapshot())
	}
}
