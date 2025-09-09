package logger

import (
	"github.com/alextorq/dns-filter/logger/handlers/console"
	"sync"
)

var onceChan sync.Once

func GetLogger() ChanLogger {
	onceChan.Do(func() {
		logChannel = NewChanLogger(1000)

		//lokiHandler := &loki.LokiHandler{
		//	URL:    "http://localhost:3100/loki/api/v1/push",
		//	Labels: `{job="news", env="local"}`,
		//}
		consoleHandler := &console.ConsoleHandler{}

		//logChannel.AddHandler(lokiHandler)
		logChannel.AddHandler(consoleHandler)
	})
	return *logChannel
}
