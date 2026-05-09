package web

import blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"

// ErrorResponse is a generic error payload returned by handlers.
type ErrorResponse struct {
	Message string `json:"message"`
}

// MessageResponse is a generic success payload with a status message.
type MessageResponse struct {
	Message string `json:"message"`
}

// GetAllDnsRecordsRequest filters and paginates the block list.
type GetAllDnsRecordsRequest struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Filter string `json:"filter"`
	Source string `json:"source"`
}

// GetAllDnsRecordsResponse is a page of block-list records.
type GetAllDnsRecordsResponse struct {
	List  []blocked_domain_db.BlockList `json:"list"`
	Total int64                         `json:"total"`
}

// UpdateDnsRecordResponse is the result of toggling a block-list record.
type UpdateDnsRecordResponse struct {
	Message string                       `json:"message"`
	Record  *blocked_domain_db.BlockList `json:"record"`
}

// GetAmountResponse reports the total number of block events.
type GetAmountResponse struct {
	Amount int64 `json:"amount"`
}

// GetAmountByDomainResponse groups block events by domain.
type GetAmountByDomainResponse struct {
	Groups []blocked_domain_db.DomainCount `json:"groups"`
}
