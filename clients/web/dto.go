package web

import "github.com/alextorq/dns-filter/clients/db"

type ErrorResponse struct {
	Error string `json:"error"`
}

type BadRequestResponse struct {
	Message string `json:"message"`
}

type StatusResponse struct {
	Status string `json:"status"`
}

type ListClientsResponse struct {
	List  []db.Client `json:"list"`
	Total int         `json:"total"`
}

type ClientResponse struct {
	Client db.Client `json:"client"`
}

// CreateClientRequest registers a new client. Exactly one identifier is
// required for LAN deployments today (IP); MAC and Token are accepted so the
// schema is stable across the planned discovery and public-mode features.
//
// Filtered is a pointer so we can distinguish "client explicitly opted into
// excluded state" from "field omitted, take the default". Without the pointer
// a JSON body of {"ip":"1.2.3.4"} would deserialize to Filtered=false (Go
// zero value) and silently create the row as an exclusion — the opposite of
// the schema default.
type CreateClientRequest struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Token    string `json:"token"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Vendor   string `json:"vendor"`
	Filtered *bool  `json:"filtered,omitempty"`
}

// UpdateClientRequest patches metadata on an existing client. nil pointers
// leave fields untouched; pointer to "" explicitly clears the field. The
// Filtered flag is not patchable here — it has its own endpoint because the
// in-memory exclusion store has to be updated in lock-step.
type UpdateClientRequest struct {
	ID       uint    `json:"id"`
	Name     *string `json:"name,omitempty"`
	Hostname *string `json:"hostname,omitempty"`
	Vendor   *string `json:"vendor,omitempty"`
}

type ChangeFilterRequest struct {
	ID       uint `json:"id"`
	Filtered bool `json:"filtered"`
}

type DeleteClientRequest struct {
	ID uint `json:"id"`
}

// DiscoverRequest controls a LAN scan. FilterDocker is a pointer so an absent
// field defaults to true (hide Docker bridges) rather than Go's false zero
// value — the UI checkbox defaults to on, and a client that omits the field
// should get the same safe default. Set it to false to include Docker
// neighbours (the "show Docker networks" toggle).
type DiscoverRequest struct {
	FilterDocker *bool `json:"filter_docker,omitempty"`
}
