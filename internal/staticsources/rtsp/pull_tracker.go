package rtsp

import (
	"sync"
	"time"
)

// lagEWMATimeConstant is the time constant of the exponential moving
// average used to smooth the lag value.
const lagEWMATimeConstant = 10 * time.Second

// pullTrackTracker tracks delivery lag of a single track.
type pullTrackTracker struct {
	clockRate float64

	mutex         sync.Mutex
	initialized   bool
	firstWall     time.Time
	lastWall      time.Time
	lastTS        uint32
	unwrappedTS   int64 // RTP timestamp progress since the first packet
	maxUnwrapped  int64 // monotonic maximum (B-frame reordering)
	smoothedLag   float64
	lagInitialize bool
}

func (t *pullTrackTracker) onPacket(ts uint32, now time.Time) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.initialized {
		t.initialized = true
		t.firstWall = now
		t.lastWall = now
		t.lastTS = ts
		return
	}

	// unwrap the 32-bit RTP timestamp; B-frame reordering makes raw
	// timestamps non-monotonic, so keep the monotonic maximum.
	t.unwrappedTS += int64(int32(ts - t.lastTS))
	t.lastTS = ts
	if t.unwrappedTS > t.maxUnwrapped {
		t.maxUnwrapped = t.unwrappedTS
	}

	mediaElapsed := float64(t.maxUnwrapped) / t.clockRate
	lag := now.Sub(t.firstWall).Seconds() - mediaElapsed
	if lag < 0 {
		// clock skew / startup can make it slightly negative
		lag = 0
	}

	if !t.lagInitialize {
		t.lagInitialize = true
		t.smoothedLag = lag
	} else {
		// time-aware EWMA: weight by the interval since the last sample
		dt := now.Sub(t.lastWall).Seconds()
		alpha := dt / (dt + lagEWMATimeConstant.Seconds())
		t.smoothedLag += alpha * (lag - t.smoothedLag)
	}

	t.lastWall = now
}

func (t *pullTrackTracker) lag() (float64, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.lagInitialize {
		return 0, false
	}
	return t.smoothedLag, true
}

// pullTracker tracks last-packet time and delivery lag of a pull source.
type pullTracker struct {
	mutex          sync.Mutex
	lastPacketTime time.Time
	tracks         []*pullTrackTracker
}

func (r *pullTracker) addTrack(clockRate int) *pullTrackTracker {
	tt := &pullTrackTracker{clockRate: float64(clockRate)}

	r.mutex.Lock()
	r.tracks = append(r.tracks, tt)
	r.mutex.Unlock()

	return tt
}

func (r *pullTracker) onPacket(tt *pullTrackTracker, ts uint32, now time.Time) {
	r.mutex.Lock()
	if now.After(r.lastPacketTime) {
		r.lastPacketTime = now
	}
	r.mutex.Unlock()

	if tt.clockRate > 0 {
		tt.onPacket(ts, now)
	}
}

// stats returns the wallclock time of the most recent packet and the
// maximum smoothed lag across tracks.
func (r *pullTracker) stats() (lastPacket *time.Time, lagSeconds *float64) {
	r.mutex.Lock()
	if !r.lastPacketTime.IsZero() {
		v := r.lastPacketTime
		lastPacket = &v
	}
	tracks := r.tracks
	r.mutex.Unlock()

	for _, tt := range tracks {
		if lag, ok := tt.lag(); ok {
			if lagSeconds == nil || lag > *lagSeconds {
				v := lag
				lagSeconds = &v
			}
		}
	}

	return lastPacket, lagSeconds
}
