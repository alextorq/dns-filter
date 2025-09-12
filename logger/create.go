package logger

import (
	"sync"

	"github.com/alextorq/dns-filter/logger/handlers/console"
)

var onceChan sync.Once
var logChannel *ChanLogger = nil

func GetLogger() *ChanLogger {
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
	return logChannel
}
