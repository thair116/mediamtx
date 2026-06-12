package webrtc

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/rtpsender"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// OutboundTrack is an outgoing track.
type OutboundTrack struct {
	Caps webrtc.RTPCodecCapability

	track      *webrtc.TrackLocalStaticRTP
	ssrc       uint32
	rtcpSender *rtpsender.Sender

	// loss and jitter reported by the remote receiver through RTCP receiver reports
	remoteReportedLost   atomic.Uint64 // maximum seen so far; monotonically non-decreasing
	remoteReportedJitter atomic.Uint32 // in clock rate units
	remoteReportPresent  atomic.Bool
}

func (t *OutboundTrack) isVideo() bool {
	return strings.Split(t.Caps.MimeType, "/")[0] == "video"
}

func (t *OutboundTrack) setup(p *PeerConnection) error {
	var trackID string
	if t.isVideo() {
		trackID = "video"
	} else {
		trackID = "audio"
	}

	var err error
	t.track, err = webrtc.NewTrackLocalStaticRTP(
		t.Caps,
		trackID,
		webrtcStreamID,
	)
	if err != nil {
		return err
	}

	sender, err := p.wr.AddTrack(t.track)
	if err != nil {
		return err
	}

	t.ssrc = uint32(sender.GetParameters().Encodings[0].SSRC)

	t.rtcpSender = &rtpsender.Sender{
		ClockRate: int(t.track.Codec().ClockRate),
		Period:    1 * time.Second,
		TimeNow:   time.Now,
		WritePacketRTCP: func(pkt rtcp.Packet) {
			p.wr.WriteRTCP([]rtcp.Packet{pkt}) //nolint:errcheck
		},
	}
	t.rtcpSender.Initialize()

	// incoming RTCP packets must always be read to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err2 := sender.Read(buf)
			if err2 != nil {
				return
			}

			pkts, err2 := rtcp.Unmarshal(buf[:n])
			if err2 != nil {
				panic(err2)
			}

			t.handleIncomingRTCP(pkts)
		}
	}()

	return nil
}

// handleIncomingRTCP extracts loss and jitter related to this track
// from RTCP receiver reports sent by the remote receiver.
func (t *OutboundTrack) handleIncomingRTCP(pkts []rtcp.Packet) {
	for _, pkt := range pkts {
		var reports []rtcp.ReceptionReport

		switch rtcpPkt := pkt.(type) {
		case *rtcp.ReceiverReport:
			reports = rtcpPkt.Reports
		case *rtcp.SenderReport:
			reports = rtcpPkt.Reports
		}

		for _, report := range reports {
			if report.SSRC == t.ssrc {
				// TotalLost can decrease when retransmitted packets are counted
				// as duplicates; keep the maximum so that the value exposed
				// through the API is monotonically non-decreasing.
				// this is the only goroutine writing remoteReportedLost.
				if v := uint64(report.TotalLost); v > t.remoteReportedLost.Load() {
					t.remoteReportedLost.Store(v)
				}
				t.remoteReportedJitter.Store(report.Jitter)
				t.remoteReportPresent.Store(true)
			}
		}
	}
}

// remoteStats returns loss and jitter reported by the remote receiver.
// Jitter is converted into seconds. ok is false if no receiver report
// has been received yet.
func (t *OutboundTrack) remoteStats() (lost uint64, jitter float64, ok bool) {
	if !t.remoteReportPresent.Load() {
		return 0, 0, false
	}

	lost = t.remoteReportedLost.Load()

	if clockRate := t.Caps.ClockRate; clockRate != 0 {
		jitter = float64(t.remoteReportedJitter.Load()) / float64(clockRate)
	}

	return lost, jitter, true
}

func (t *OutboundTrack) close() {
	if t.rtcpSender != nil {
		t.rtcpSender.Close()
	}
}

// WriteRTP writes a RTP packet.
func (t *OutboundTrack) WriteRTP(pkt *rtp.Packet) error {
	return t.WriteRTPWithNTP(pkt, time.Now())
}

// WriteRTPWithNTP writes a RTP packet.
func (t *OutboundTrack) WriteRTPWithNTP(pkt *rtp.Packet, ntp time.Time) error {
	// use right SSRC in packet to make rtcpSender work
	pkt.SSRC = t.ssrc

	t.rtcpSender.ProcessPacket(pkt, ntp, true)

	return t.track.WriteRTP(pkt)
}
