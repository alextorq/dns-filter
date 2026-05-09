package web

import (
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// DownloadDb streams the SQLite database file as an attachment.
// @Summary      Download database file
// @Tags         config
// @Produce      application/octet-stream
// @Success      200 {file} binary "filter.sqlite"
// @Router       /api/config/db/download [get]
func DownloadDb(c *gin.Context) {
	l := logger.GetLogger()
	conf := config.GetConfig()
	l.Info("Downloading database file: " + conf.DbPath)
	c.FileAttachment(conf.DbPath, "filter.sqlite")
}
