package db

import (
	"fmt"
	"os"
	"time"

	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/metric"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	sqliteFileSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sqlite_file_size_bytes",
		Help: "Actual SQLite file size in bytes",
	})
)

func GetDbSize() {
	l := logger.GetLogger()
	info, err := os.Stat(GetDBConnectionString())
	if err != nil {
		fmt.Println(fmt.Errorf("failed to get file info: %w", err))
		return
	}
	size := info.Size()
	sqliteFileSize.Set(float64(info.Size()))
	l.Info(fmt.Sprintf("Database size: %f bytes", float64(size)/1024.0/1024.0))
}

func MonitoringDbSize() {
	ticker := time.NewTicker(10 * time.Minute)
	GetDbSize()
	for range ticker.C {
		GetDbSize()
	}
}

func init() {
	metric.Registry.MustRegister(sqliteFileSize)
	go MonitoringDbSize()
}
