package web

import suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"

type ErrorResponse struct {
	Message string `json:"message"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type GetAllSuggestBlocksRequest struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Filter string `json:"filter"`
	Active *bool  `json:"active"`
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
