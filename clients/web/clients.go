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

func GetAllClients(c *gin.Context) {
	clients, err := db.GetAllClients()
	if err != nil {
		c.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"list":  clients,
		"total": len(clients),
	})
}

type AddClientRequest struct {
	UserId string `json:"user_id"`
}

func AddClient(c *gin.Context) {
	var req AddClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := add.AddClient(req.UserId)
	if err != nil {
		c.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"status": "ok",
	})
}

type ChangeClientStatusRequest struct {
	ID       uint `json:"id"`
	IsActive bool `json:"is_active"`
}

func ChangeClientStatus(c *gin.Context) {
	var req ChangeClientStatusRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := db.UpdateClientIsActive(req.ID, req.IsActive)
	if err != nil {
		c.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"status": "ok",
	})
}

type DeleteClientRequest struct {
	ID uint `json:"id"`
}

func DeleteClient(c *gin.Context) {
	var req DeleteClientRequest
	l := logger.GetLogger()

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := remove.RemoveClient(req.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"status": "ok",
	})
}
