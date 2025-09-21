package web

import (
	"github.com/alextorq/dns-filter/black-lists/web"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CreateSever() *gin.Engine {
	r := gin.Default()
	//TODO: configure CORS
	r.Use(cors.Default())

	r.POST("/api/dns-records", web.GetAllDnsRecords)
	r.POST("/api/dns-records/create", web.CreateDnsRecords)
	r.POST("/api/dns-records/update", web.ChangeDnsRecordActive)

	r.GET("/api/filter/status", filterWeb.GetFilterStatus)
	r.POST("/api/filter/change-status", filterWeb.ChangeFilterStatus)

	go func() {
		r.Run()
	}()

	return r
}
