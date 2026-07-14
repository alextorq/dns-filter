package web

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Logger is the narrow logging port used by the database download handler.
type Logger interface {
	Info(args ...any)
	Error(err error)
}

// SnapshotExporter builds a sanitized, point-in-time database snapshot. The
// returned reader owns all temporary resources; callers must close it after the
// response has been streamed. Production uses the SQLite/filesystem adapter in
// db/snapshot, while handler tests inject a memory-backed fake.
type SnapshotExporter interface {
	Export(ctx context.Context) (content io.ReadCloser, size int64, err error)
}

// Handlers owns only the HTTP-facing dependencies of the database feature.
// SQLite, filesystem and secret-sanitization details live behind Exporter.
type Handlers struct {
	Exporter SnapshotExporter
	Log      Logger
}

// DownloadDb streams a sanitized snapshot of the SQLite database as an
// attachment. Snapshot creation and cleanup are delegated to SnapshotExporter;
// this method only maps failures to HTTP and transfers the result.
// @Summary      Download database file
// @Tags         config
// @Produce      application/octet-stream
// @Success      200 {file} binary "filter.sqlite"
// @Failure      500 {object} map[string]string
// @Router       /api/config/db/download [get]
func (h *Handlers) DownloadDb(c *gin.Context) {
	if err := h.validate(); err != nil {
		if h != nil && h.Log != nil {
			h.Log.Error(fmt.Errorf("download db: invalid handler wiring: %w", err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}

	content, size, err := h.Exporter.Export(c.Request.Context())
	if err != nil {
		h.Log.Error(fmt.Errorf("download db: export snapshot: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}
	if content == nil || size < 0 {
		if content != nil {
			_ = content.Close()
		}
		h.Log.Error(fmt.Errorf("download db: exporter returned invalid snapshot"))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "snapshot failed"})
		return
	}
	defer func() {
		if err := content.Close(); err != nil {
			h.Log.Error(fmt.Errorf("download db: close snapshot: %w", err))
		}
	}()

	h.Log.Info("Downloading sanitized database snapshot")
	c.DataFromReader(
		http.StatusOK,
		size,
		"application/octet-stream",
		content,
		map[string]string{"Content-Disposition": `attachment; filename="filter.sqlite"`},
	)
}

// validate makes incomplete DI fail closed. Missing exporter wiring must never
// degrade to serving the live, unsanitized database file.
func (h *Handlers) validate() error {
	if h == nil {
		return fmt.Errorf("handlers are nil")
	}
	if h.Exporter == nil {
		return fmt.Errorf("snapshot exporter is nil")
	}
	if h.Log == nil {
		return fmt.Errorf("logger is nil")
	}
	return nil
}
