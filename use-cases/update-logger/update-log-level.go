package update_logger

import (
	"github.com/alextorq/dns-filter/logger"
)

func UpdateLogLevel(level string) {
	l := logger.GetLogger()
	l.UpdateLogLevel(level)
	l.Info("Log level updated to: " + level)
}
