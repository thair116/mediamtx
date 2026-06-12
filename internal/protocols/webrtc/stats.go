package webrtc

// Stats are WebRTC statistics.
type Stats struct {
	BytesReceived      uint64
	BytesSent          uint64
	RTPPacketsReceived uint64
	RTPPacketsSent     uint64
	RTPPacketsLost     uint64
	RTPPacketsJitter   float64
	// loss and jitter (in seconds) related to outbound tracks,
	// as reported by the remote receiver through RTCP receiver reports.
	// RTPPacketsReportedLost is monotonically non-decreasing.
	RTPPacketsReportedLost   uint64
	RTPPacketsReportedJitter float64
	// NACK and PLI packets received from the remote receiver,
	// related to outbound tracks. Cumulative, never decreasing.
	NACKsReceived       uint64
	PLIsReceived        uint64
	RTCPPacketsReceived uint64
	RTCPPacketsSent     uint64
}
