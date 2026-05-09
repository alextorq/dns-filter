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

type GetAllClientsResponse struct {
	List  []db.ExcludeClient `json:"list"`
	Total int                `json:"total"`
}
