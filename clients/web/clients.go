package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/clients"
	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/use-cases/create"
	"github.com/alextorq/dns-filter/clients/use-cases/update"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// ListClients lists every known client.
// @Summary      List clients
// @Tags         clients
// @Produce      json
// @Success      200 {object} ListClientsResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/clients [post]
func ListClients(c *gin.Context) {
	rows, err := db.GetAllClients()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListClientsResponse{List: rows, Total: len(rows)})
}

// CreateClient registers a new client.
// @Summary      Create client
// @Tags         clients
// @Accept       json
// @Produce      json
// @Param        body body     CreateClientRequest true "Client to create"
// @Success      200  {object} ClientResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/clients/create [post]
func CreateClient(c *gin.Context) {
	var req CreateClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("create client: bind json: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	// Default to Filtered=true (matches the schema default) when the field is
	// omitted. The pointer indirection in the request DTO exists for exactly
	// this distinction — see the CreateClientRequest doc comment.
	filtered := boolOr(req.Filtered, true)
	row, err := clients.Create(create.Input{
		IP:       req.IP,
		MAC:      req.MAC,
		Token:    req.Token,
		Name:     req.Name,
		Hostname: req.Hostname,
		Vendor:   req.Vendor,
		Filtered: filtered,
	})
	if err != nil {
		if errors.Is(err, create.ErrNoIdentifier) {
			c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ClientResponse{Client: *row})
}

// UpdateClient patches metadata on an existing client.
// @Summary      Update client metadata
// @Tags         clients
// @Accept       json
// @Produce      json
// @Param        body body     UpdateClientRequest true "Fields to patch"
// @Success      200  {object} ClientResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/clients/update [post]
func UpdateClient(c *gin.Context) {
	var req UpdateClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("update client: bind json: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	row, err := clients.Update(update.Input{
		ID:       req.ID,
		Name:     req.Name,
		Hostname: req.Hostname,
		Vendor:   req.Vendor,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ClientResponse{Client: *row})
}

// ChangeFilter toggles whether DNS filtering applies to a client.
// @Summary      Change client filter flag
// @Tags         clients
// @Accept       json
// @Produce      json
// @Param        body body     ChangeFilterRequest true "Target state"
// @Success      200  {object} ClientResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/clients/change-filter [post]
func ChangeFilter(c *gin.Context) {
	var req ChangeFilterRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("change filter: bind json: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	row, err := clients.ChangeFilter(req.ID, req.Filtered)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, ClientResponse{Client: *row})
}

// DeleteClient removes a client.
// @Summary      Delete client
// @Tags         clients
// @Accept       json
// @Produce      json
// @Param        body body     DeleteClientRequest true "Client id"
// @Success      200  {object} StatusResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/clients/delete [post]
func DeleteClient(c *gin.Context) {
	var req DeleteClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("delete client: bind json: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	if err := clients.Remove(req.ID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, StatusResponse{Status: "ok"})
}
