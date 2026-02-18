package web

import (
	"github.com/alextorq/dns-filter/clients/db"
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
	c.JSON(200, clients)
}

type AddClientRequest struct {
	UserId uint `json:"user_id"`
}

func AddClient(c *gin.Context) {
	var client AddClientRequest
	err := c.BindJSON(&client)
	if err != nil {
		c.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = db.AddClient(client.UserId)
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
