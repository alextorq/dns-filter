package web

import (
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

func DownloadDb(c *gin.Context) {
	l := logger.GetLogger()
	conf := config.GetConfig()
	l.Info("Downloading database file: " + conf.DbPath)
	c.FileAttachment(conf.DbPath, "filter.sqlite")
}
