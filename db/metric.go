package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const DefaultDBSizeMonitorInterval = 10 * time.Minute

type DBSizeMonitorLogger interface {
	Debug(args ...any)
	Error(error)
}

type statFileFunc func(string) (os.FileInfo, error)

// DBSizeMonitor periodically publishes the SQLite file size. It owns no
// goroutine: main runs Run with the application context and can wait for it
// during shutdown.
type DBSizeMonitor struct {
	path     string
	log      DBSizeMonitorLogger
	interval time.Duration
	stat     statFileFunc
	gauge    prometheus.Gauge
}

func NewDBSizeMonitor(
	registerer prometheus.Registerer,
	path string,
	log DBSizeMonitorLogger,
	interval time.Duration,
) (*DBSizeMonitor, error) {
	return newDBSizeMonitor(registerer, path, log, interval, os.Stat)
}

func newDBSizeMonitor(
	registerer prometheus.Registerer,
	path string,
	log DBSizeMonitorLogger,
	interval time.Duration,
	stat statFileFunc,
) (*DBSizeMonitor, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("DB size monitor interval must be positive: %s", interval)
	}
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sqlite_file_size_bytes",
		Help: "Actual SQLite file size in bytes",
	})
	if err := registerer.Register(gauge); err != nil {
		return nil, fmt.Errorf("register sqlite file size metric: %w", err)
	}
	return &DBSizeMonitor{
		path:     path,
		log:      log,
		interval: interval,
		stat:     stat,
		gauge:    gauge,
	}, nil
}

// Run observes once immediately, then on every interval until ctx is canceled.
// A pre-canceled context performs no filesystem access.
func (m *DBSizeMonitor) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	m.observe()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.observe()
		}
	}
}

func (m *DBSizeMonitor) observe() {
	info, err := m.stat(m.path)
	if err != nil {
		m.log.Error(fmt.Errorf("read database file size: %w", err))
		return
	}
	size := info.Size()
	m.gauge.Set(float64(size))
	m.log.Debug(fmt.Sprintf("Database size: %f MiB", float64(size)/1024.0/1024.0))
}
