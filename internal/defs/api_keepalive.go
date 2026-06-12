package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIKeepalive is a keepalive.
type APIKeepalive struct {
	ID          uuid.UUID `json:"id"`
	Created     time.Time `json:"created"`
	Path        string    `json:"path"`
	CreatorUser string    `json:"creatorUser"`
	CreatorIP   string    `json:"creatorIP"`
}

// APIKeepaliveList is a list of keepalives.
type APIKeepaliveList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []*APIKeepalive `json:"items"`
}
