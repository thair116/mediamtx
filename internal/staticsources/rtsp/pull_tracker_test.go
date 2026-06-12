package rtsp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPullTrackerLastPacketTime(t *testing.T) {
	r := &pullTracker{}
	tt := r.addTrack(90000)

	last, lag := r.stats()
	require.Nil(t, last)
	require.Nil(t, lag)

	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	r.onPacket(tt, 0, now)

	last, _ = r.stats()
	require.NotNil(t, last)
	require.Equal(t, now, *last)

	// an out-of-order wallclock must not move lastPacketTime backwards
	r.onPacket(tt, 3000, now.Add(-time.Second))
	last, _ = r.stats()
	require.Equal(t, now, *last)
}

func TestPullTrackerLagPacedSource(t *testing.T) {
	// a live source paced at 1x: media time advances together with
	// wallclock; lag must stay near zero.
	r := &pullTracker{}
	tt := r.addTrack(90000)

	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	for i := range 100 {
		ts := uint32(i * 3000) // 30 fps
		r.onPacket(tt, ts, start.Add(time.Duration(i)*time.Second/30))
	}

	_, lag := r.stats()
	require.NotNil(t, lag)
	require.Less(t, *lag, 0.05)
}

func TestPullTrackerLagDelayedDelivery(t *testing.T) {
	// delivery falls 2 seconds behind: wallclock advances 2s more than
	// media time. The EWMA must converge near 2s.
	r := &pullTracker{}
	tt := r.addTrack(90000)

	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	wall := start

	// 5 seconds healthy
	for i := range 150 {
		r.onPacket(tt, uint32(i*3000), wall)
		wall = wall.Add(time.Second / 30)
	}

	// stall: no packets for 2 seconds, then delivery resumes at 1x but
	// permanently 2 seconds behind
	wall = wall.Add(2 * time.Second)
	for i := 150; i < 1500; i++ {
		r.onPacket(tt, uint32(i*3000), wall)
		wall = wall.Add(time.Second / 30)
	}

	_, lag := r.stats()
	require.NotNil(t, lag)
	require.InDelta(t, 2.0, *lag, 0.5)
}

func TestPullTrackerLagClampAndReordering(t *testing.T) {
	// B-frame reordering: timestamps jump backwards; media progress must
	// use the monotonic maximum and lag must never go negative.
	r := &pullTracker{}
	tt := r.addTrack(90000)

	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	wall := start

	seq := []uint32{0, 9000, 3000, 6000, 18000, 12000, 15000}
	for _, ts := range seq {
		r.onPacket(tt, ts, wall)
		wall = wall.Add(time.Second / 30)
	}

	_, lag := r.stats()
	require.NotNil(t, lag)
	require.GreaterOrEqual(t, *lag, 0.0)
}

func TestPullTrackerTimestampWraparound(t *testing.T) {
	// RTP timestamps are 32-bit and wrap; unwrapping must keep media
	// progress correct across the wrap.
	r := &pullTracker{}
	tt := r.addTrack(90000)

	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	wall := start

	ts := uint32(0xFFFFFFFF - 30000)
	for range 100 {
		r.onPacket(tt, ts, wall)
		ts += 3000 // wraps mid-loop
		wall = wall.Add(time.Second / 30)
	}

	_, lag := r.stats()
	require.NotNil(t, lag)
	require.Less(t, *lag, 0.05)
}

func TestPullTrackerMaxAcrossTracks(t *testing.T) {
	r := &pullTracker{}
	video := r.addTrack(90000)
	audio := r.addTrack(48000)

	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	// video on time, audio 3 seconds behind
	for i := range 600 {
		wall := start.Add(time.Duration(i) * time.Second / 30)
		r.onPacket(video, uint32(i*3000), wall)
		audioElapsed := float64(i)/30 - 3
		if audioElapsed < 0 {
			audioElapsed = 0
		}
		r.onPacket(audio, uint32(audioElapsed*48000), wall)
	}

	_, lag := r.stats()
	require.NotNil(t, lag)
	require.InDelta(t, 3.0, *lag, 0.5)
}
