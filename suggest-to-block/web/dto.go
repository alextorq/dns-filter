package web

import (
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type GetAllSuggestBlocksRequest struct {
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
	Filter string   `json:"filter"`
	Active *bool    `json:"active"`
	Codes  []string `json:"codes"`
}

type GetSignalCodesResponse struct {
	List []collect.SignalDescriptor `json:"list"`
}

type GetAllSuggestBlocksResponse struct {
	List  []suggest_to_block_db.SuggestBlock `json:"list"`
	Total int64                              `json:"total"`
}

type AddToBlockRequest struct {
	ID     uint   `json:"id"`
	Domain string `json:"domain"`
}

type ChangeSuggestStatusRequest struct {
	ID     uint `json:"id"`
	Active bool `json:"active"`
}
