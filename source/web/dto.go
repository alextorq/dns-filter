package web

import syncDb "github.com/alextorq/dns-filter/source/db"

type ErrorResponse struct {
	Message string `json:"message"`
}

type GetAllSourcesResponse struct {
	List  []syncDb.Source `json:"list"`
	Total int64           `json:"total"`
}

type ChangeSourceActiveRequest struct {
	ID     uint `json:"id"`
	Active bool `json:"active"`
}

type ChangeSourceActiveResponse struct {
	Message string         `json:"message"`
	Record  *syncDb.Source `json:"record"`
}
