package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/logger/business/use-cases/update-logger"
	"github.com/gin-gonic/gin"
)

type UpdateConfigData struct {
	LogLevel string `json:"logLevel"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type LogLevelResponse struct {
	Level string `json:"level"`
}

// ChangeLogLevel updates the runtime log level.
// @Summary      Change log level
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        body body     UpdateConfigData true "Target log level"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} MessageResponse
// @Router       /api/config/logger/change-level [post]
func ChangeLogLevel(c *gin.Context) {
	req := UpdateConfigData{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Message: "not valid payload"})
		return
	}
	if _, err := logger.LogLevelFromStringOrError(req.LogLevel); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Message: err.Error()})
		return
	}
	update_logger.UpdateLogLevel(req.LogLevel)
	c.JSON(http.StatusOK, MessageResponse{Message: "done"})
}

// GetLogLevel reports the current runtime log level.
// @Summary      Get log level
// @Tags         config
// @Produce      json
// @Success      200 {object} LogLevelResponse
// @Router       /api/config/logger/get-level [post]
func GetLogLevel(c *gin.Context) {
	l := logger.GetLogger()
	c.JSON(http.StatusOK, LogLevelResponse{Level: l.GetLogLevel()})
}
