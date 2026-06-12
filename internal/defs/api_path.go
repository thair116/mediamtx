package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIPathManager contains methods used by the API and Metrics server.
type APIPathManager interface {
	APIPathsList() (*APIPathList, error)
	APIPathsGet(string) (*APIPath, error)
	APIKeepaliveAdd(PathAccessRequest) (uuid.UUID, error)
	APIKeepaliveRemove(uuid.UUID, PathAccessRequest) error
	APIKeepalivesList() (*APIKeepaliveList, error)
	APIKeepalivesGet(uuid.UUID) (*APIKeepalive, error)
}

// APIPathSourceType is the type of a path source.
type APIPathSourceType string

// source types.
const (
	APIPathSourceTypeHLSSource       APIPathSourceType = "hlsSource"
	APIPathSourceTypeRedirect        APIPathSourceType = "redirect"
	APIPathSourceTypeRPICameraSource APIPathSourceType = "rpiCameraSource"
	APIPathSourceTypeRTMPConn        APIPathSourceType = "rtmpConn"
	APIPathSourceTypeRTMPSConn       APIPathSourceType = "rtmpsConn"
	APIPathSourceTypeRTMPSource      APIPathSourceType = "rtmpSource"
	APIPathSourceTypeRTSPSession     APIPathSourceType = "rtspSession"
	APIPathSourceTypeRTSPSource      APIPathSourceType = "rtspSource"
	APIPathSourceTypeRTSPSSession    APIPathSourceType = "rtspsSession"
	APIPathSourceTypeSRTConn         APIPathSourceType = "srtConn"
	APIPathSourceTypeSRTSource       APIPathSourceType = "srtSource"
	APIPathSourceTypeMPEGTSSource    APIPathSourceType = "mpegtsSource"
	APIPathSourceTypeRTPSource       APIPathSourceType = "rtpSource"
	APIPathSourceTypeWebRTCSession   APIPathSourceType = "webRTCSession"
	APIPathSourceTypeWebRTCSource    APIPathSourceType = "webRTCSource"
	APIPathSourceTypeMoQSession      APIPathSourceType = "moqSession"
)

// APIPathSource is a source.
type APIPathSource struct {
	Type APIPathSourceType `json:"type"`
	ID   string            `json:"id"`

	// statistics of the connection between this server and the source.
	// They are currently populated for RTSP/RTSPS pull sources only;
	// null for other source types.
	InboundBytes            *uint64  `json:"inboundBytes"`
	InboundRTPPackets       *uint64  `json:"inboundRTPPackets"`
	InboundRTPPacketsLost   *uint64  `json:"inboundRTPPacketsLost"`
	InboundRTPPacketsJitter *float64 `json:"inboundRTPPacketsJitter"`
	// wallclock time of the most recent RTP packet on any track.
	LastPacketTime *time.Time `json:"lastPacketTime"`
	// how far delivery is behind real time, in seconds (EWMA-smoothed).
	SourceLagSeconds *float64 `json:"sourceLagSeconds"`
}

// APIPathReaderType is the type of a path reader.
type APIPathReaderType string

// reader types.
const (
	APIPathReaderTypeHLSSession    APIPathReaderType = "hlsSession"
	APIPathReaderTypeRTMPConn      APIPathReaderType = "rtmpConn"
	APIPathReaderTypeRTMPSConn     APIPathReaderType = "rtmpsConn"
	APIPathReaderTypeRTSPConn      APIPathReaderType = "rtspConn"
	APIPathReaderTypeRTSPSession   APIPathReaderType = "rtspSession"
	APIPathReaderTypeRTSPSConn     APIPathReaderType = "rtspsConn"
	APIPathReaderTypeRTSPSSession  APIPathReaderType = "rtspsSession"
	APIPathReaderTypeSRTConn       APIPathReaderType = "srtConn"
	APIPathReaderTypeWebRTCSession APIPathReaderType = "webRTCSession"
	APIPathReaderTypeMoQSession    APIPathReaderType = "moqSession"
	APIPathReaderTypeHidden        APIPathReaderType = "hidden"
	APIPathReaderTypeKeepalive     APIPathReaderType = "keepalive"
)

// APIPathReader is a reader.
type APIPathReader struct {
	Type APIPathReaderType `json:"type"`
	ID   string            `json:"id"`
}

// APIPath is a path.
type APIPath struct {
	Name                 string              `json:"name"`
	ConfName             string              `json:"confName"`
	Ready                bool                `json:"ready" deprecated:"true"`
	ReadyTime            *time.Time          `json:"readyTime" deprecated:"true"`
	Available            bool                `json:"available"`
	AvailableTime        *time.Time          `json:"availableTime"`
	Online               bool                `json:"online"`
	OnlineTime           *time.Time          `json:"onlineTime"`
	Source               *APIPathSource      `json:"source"`
	Tracks               []APIPathTrackCodec `json:"tracks" deprecated:"true"`
	Tracks2              []APIPathTrack      `json:"tracks2"`
	Readers              []APIPathReader     `json:"readers"`
	InboundBytes         uint64              `json:"inboundBytes"`
	OutboundBytes        uint64              `json:"outboundBytes"`
	InboundFramesInError uint64              `json:"inboundFramesInError"`
	// deprecated
	BytesReceived uint64 `json:"bytesReceived" deprecated:"true"`
	BytesSent     uint64 `json:"bytesSent" deprecated:"true"`
}

// APIPathList is a list of paths.
type APIPathList struct {
	ItemCount int       `json:"itemCount"`
	PageCount int       `json:"pageCount"`
	Items     []APIPath `json:"items"`
}
