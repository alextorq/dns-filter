package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// Handlers groups the runtime-logger HTTP endpoints with their dependencies.
//
// SetLogLevel persists and applies a new level — it is wired to the settings
// module at the composition root so a level set here survives a restart (the
// historical /api/config/logger/change-level endpoint used to mutate the
// logger in memory only). GetLogLevel reports the current effective level.
type Handlers struct {
	SetLogLevel func(level string) error
	GetLogLevel func() string
}

type UpdateConfigData struct {
	LogLevel string `json:"logLevel"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type LogLevelResponse struct {
	Level string `json:"level"`
}

// ChangeLogLevel persists and applies the runtime log level.
// @Summary      Change log level
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        body body     UpdateConfigData true "Target log level"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} MessageResponse
// @Failure      500  {object} MessageResponse
// @Router       /api/config/logger/change-level [post]
func (h *Handlers) ChangeLogLevel(c *gin.Context) {
	req := UpdateConfigData{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Message: "not valid payload"})
		return
	}
	// Validate here for a precise client-facing message; the settings
	// descriptor validates again before persisting (defense in depth).
	if _, err := logger.LogLevelFromStringOrError(req.LogLevel); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Message: err.Error()})
		return
	}
	if err := h.SetLogLevel(req.LogLevel); err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Message: "failed to persist log level"})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "done"})
}

// GetLogLevel reports the current runtime log level.
// @Summary      Get log level
// @Tags         config
// @Produce      json
// @Success      200 {object} LogLevelResponse
// @Router       /api/config/logger/get-level [post]
func (h *Handlers) GetLogLevelHandler(c *gin.Context) {
	c.JSON(http.StatusOK, LogLevelResponse{Level: h.GetLogLevel()})
}
