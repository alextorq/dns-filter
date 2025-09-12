package metric

import (
	"net/http"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var conf = config.GetConfig()
var l = logger.GetLogger()

var Registry = prometheus.NewRegistry()

func init() {
	// подключаем стандартные
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	if conf.MetricEnable {
		m := &Metrics{
			Enable: conf.MetricEnable,
			Port:   conf.MetricPort,
		}
		m.Serve()
	}
}

type Metrics struct {
	Enable bool
	Port   string
}

func (m *Metrics) Serve() *Metrics {
	pathListen := ":" + m.Port

	l.Info("Метрики Prometheus доступны на", pathListen+"/metrics")
	// Запускаем HTTP сервер для метрик в отдельной горутине
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(pathListen, nil)
		if err != nil {
			l.Error(err)
		}
	}()
	return m
}
