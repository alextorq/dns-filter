// Package create handles registration of a new client. It validates the
// input, persists the row, and registers any non-empty identifier in the
// in-memory exclusion store when the client is created with Filtered=false.
package create

import (
	"errors"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/store"
)

// Input groups the user-controlled fields a caller may set on creation.
// At least one identifier (IP, MAC, or Token) must be present — without it
// there's no way for the hot path to ever match the row.
type Input struct {
	IP       string
	MAC      string
	Token    string
	Name     string
	Hostname string
	Vendor   string
	// Filtered=true is the default at the DB layer. Pass false to register a
	// client that bypasses the filter (the historical "exclude client" case).
	Filtered bool
}

// ErrNoIdentifier is returned when none of IP/MAC/Token is provided.
var ErrNoIdentifier = errors.New("client must have at least one identifier (ip, mac, or token)")

func Create(in Input) (*db.Client, error) {
	if in.IP == "" && in.MAC == "" && in.Token == "" {
		return nil, ErrNoIdentifier
	}
	c := &db.Client{
		IP:       in.IP,
		MAC:      in.MAC,
		Token:    in.Token,
		Name:     in.Name,
		Hostname: in.Hostname,
		Vendor:   in.Vendor,
		Filtered: in.Filtered,
	}
	if err := db.CreateClient(c); err != nil {
		return nil, err
	}
	if !c.Filtered {
		store.Get().AddClient(c)
	}
	return c, nil
}
