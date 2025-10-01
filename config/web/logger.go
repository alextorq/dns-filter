package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/logger"
	update_logger "github.com/alextorq/dns-filter/use-cases/update-logger"
	"github.com/gin-gonic/gin"
)

type UpdateConfigData struct {
	LogLevel string `json:"logLevel"`
}

func ChangeLogLevel(c *gin.Context) {
	req := UpdateConfigData{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "not valid payload",
		})
		return
	}
	if _, err := logger.LogLevelFromStringOrError(req.LogLevel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err,
		})
		return
	}
	update_logger.UpdateLogLevel(req.LogLevel)
	c.JSON(http.StatusOK, gin.H{
		"message": "done",
	})
}

func GetLogLevel(c *gin.Context) {
	l := logger.GetLogger()
	c.JSON(http.StatusOK, gin.H{
		"level": l.GetLogLevel(),
	})
}
