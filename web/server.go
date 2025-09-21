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

	r.POST("/dns-records", web.GetAllDnsRecords)
	r.POST("/dns-records/create", web.CreateDnsRecords)
	r.POST("/dns-records/update", web.ChangeDnsRecordActive)

	r.GET("/filter/status", filterWeb.GetFilterStatus)
	r.POST("/filter/change-status", filterWeb.ChangeFilterStatus)

	// Отдача статических файлов из /app/frontend
	r.Static("/", "./frontend")

	// Если нужен SPA fallback (Vue/React/Angular)
	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/index.html")
	})

	go func() {
		r.Run()
	}()

	return r
}
