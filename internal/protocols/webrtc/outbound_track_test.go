package webrtc

import (
	"testing"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestOutboundTrackRemoteStats(t *testing.T) {
	tr := &OutboundTrack{
		Caps: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeH264,
			ClockRate: 90000,
		},
		ssrc: 123456,
	}

	// no receiver report yet
	_, _, ok := tr.remoteStats()
	require.False(t, ok)

	// receiver report for a different SSRC is ignored
	tr.handleIncomingRTCP([]rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				SSRC:      999999,
				TotalLost: 50,
				Jitter:    900,
			}},
		},
	})
	_, _, ok = tr.remoteStats()
	require.False(t, ok)

	// receiver report for our SSRC is stored
	tr.handleIncomingRTCP([]rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				SSRC:      123456,
				TotalLost: 42,
				Jitter:    9000,
			}},
		},
	})
	lost, jitter, ok := tr.remoteStats()
	require.True(t, ok)
	require.Equal(t, uint64(42), lost)
	require.Equal(t, 0.1, jitter) // 9000 / 90000 clock rate

	// reception reports inside sender reports are handled too
	tr.handleIncomingRTCP([]rtcp.Packet{
		&rtcp.SenderReport{
			Reports: []rtcp.ReceptionReport{{
				SSRC:      123456,
				TotalLost: 100,
				Jitter:    4500,
			}},
		},
	})
	lost, jitter, ok = tr.remoteStats()
	require.True(t, ok)
	require.Equal(t, uint64(100), lost)
	require.Equal(t, 0.05, jitter)

	// a report with a lower TotalLost (RTX recovery / duplicates)
	// must not decrease the exposed value
	tr.handleIncomingRTCP([]rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				SSRC:      123456,
				TotalLost: 60,
				Jitter:    4500,
			}},
		},
	})
	lost, _, ok = tr.remoteStats()
	require.True(t, ok)
	require.Equal(t, uint64(100), lost)
}
