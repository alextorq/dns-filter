package web

import (
	"errors"
	"net/http"

	"github.com/alextorq/dns-filter/settings"
	"github.com/gin-gonic/gin"
)

// Service is the settings-module surface the HTTP layer needs. *settings.Module
// satisfies it; tests inject a fake.
type Service interface {
	List() ([]settings.Effective, error)
	Set(key, raw string) error
	Reset(key string) error
}

// Handlers groups the settings HTTP endpoints with their module dependency.
type Handlers struct {
	Service Service
}

// UpdateSettingRequest is the body of PUT /api/settings/{key}.
type UpdateSettingRequest struct {
	Value string `json:"value"`
}

// MessageResponse is a generic error/info envelope.
type MessageResponse struct {
	Message string `json:"message"`
}

// ListSettings returns the effective value and metadata of every dynamic
// setting, so the UI can render a typed editor and show which values are
// operator-overridden vs. inherited from the environment.
// @Summary      List runtime settings
// @Tags         settings
// @Produce      json
// @Success      200 {array}  settings.Effective
// @Failure      500 {object} MessageResponse
// @Router       /api/settings [get]
func (h *Handlers) ListSettings(c *gin.Context) {
	list, err := h.Service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, MessageResponse{Message: "failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, list)
}

// UpdateSetting validates, persists and applies a new value for one setting.
// @Summary      Update a runtime setting
// @Tags         settings
// @Accept       json
// @Produce      json
// @Param        key  path     string               true "setting key"
// @Param        body body     UpdateSettingRequest true "new value"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} MessageResponse
// @Failure      404  {object} MessageResponse
// @Failure      500  {object} MessageResponse
// @Router       /api/settings/{key} [put]
func (h *Handlers) UpdateSetting(c *gin.Context) {
	key := c.Param("key")
	var req UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Message: "invalid body"})
		return
	}
	if err := h.Service.Set(key, req.Value); err != nil {
		c.JSON(statusForError(err), MessageResponse{Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "done"})
}

// ResetSetting deletes the override for a setting, reverting it to the
// environment/compiled default.
// @Summary      Reset a runtime setting to its default
// @Tags         settings
// @Produce      json
// @Param        key path     string true "setting key"
// @Success      200 {object} MessageResponse
// @Failure      404 {object} MessageResponse
// @Failure      500 {object} MessageResponse
// @Router       /api/settings/{key} [delete]
func (h *Handlers) ResetSetting(c *gin.Context) {
	key := c.Param("key")
	if err := h.Service.Reset(key); err != nil {
		c.JSON(statusForError(err), MessageResponse{Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "done"})
}

// statusForError maps the settings module's sentinel errors to HTTP codes:
// unknown key → 404, invalid value → 400, anything else (persist/apply
// failure) → 500.
func statusForError(err error) int {
	switch {
	case errors.Is(err, settings.ErrUnknownKey):
		return http.StatusNotFound
	case errors.Is(err, settings.ErrInvalidValue):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
