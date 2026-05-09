package web

import (
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/use-cases/add"
	"github.com/alextorq/dns-filter/clients/use-cases/remove"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// GetAllClients lists all DNS clients excluded from filtering.
// @Summary      List exclude clients
// @Tags         exclude-clients
// @Produce      json
// @Success      200 {object} GetAllClientsResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/exclude-clients [post]
func GetAllClients(c *gin.Context) {
	clients, err := db.GetAllClients()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GetAllClientsResponse{
		List:  clients,
		Total: len(clients),
	})
}

type AddClientRequest struct {
	UserId string `json:"user_id"`
}

// AddClient registers a client to be excluded from filtering.
// @Summary      Add exclude client
// @Tags         exclude-clients
// @Accept       json
// @Produce      json
// @Param        body body     AddClientRequest true "Client to add"
// @Success      200  {object} StatusResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/exclude-clients/add [post]
func AddClient(c *gin.Context) {
	var req AddClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	err := add.AddClient(req.UserId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, StatusResponse{Status: "ok"})
}

type ChangeClientStatusRequest struct {
	ID       uint `json:"id"`
	IsActive bool `json:"is_active"`
}

// ChangeClientStatus toggles the active flag on an exclude-client.
// @Summary      Change exclude-client active state
// @Tags         exclude-clients
// @Accept       json
// @Produce      json
// @Param        body body     ChangeClientStatusRequest true "Target state"
// @Success      200  {object} StatusResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/exclude-clients/change-status [post]
func ChangeClientStatus(c *gin.Context) {
	var req ChangeClientStatusRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	err := db.UpdateClientIsActive(req.ID, req.IsActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, StatusResponse{Status: "ok"})
}

type DeleteClientRequest struct {
	ID uint `json:"id"`
}

// DeleteClient removes an exclude-client.
// @Summary      Delete exclude client
// @Tags         exclude-clients
// @Accept       json
// @Produce      json
// @Param        body body     DeleteClientRequest true "Client id"
// @Success      200  {object} StatusResponse
// @Failure      400  {object} BadRequestResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/exclude-clients/delete [post]
func DeleteClient(c *gin.Context) {
	var req DeleteClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
		return
	}

	err := remove.RemoveClient(req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, StatusResponse{Status: "ok"})
}
