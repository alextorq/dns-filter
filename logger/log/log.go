package log

import "time"

type LogStruct struct {
	Level   string
	Message string
	Time    time.Time
}
