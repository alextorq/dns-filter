package logger

import (
	"dns-filter/logger/log"
	"errors"
	"fmt"
	"strings"
	"time"
)

var logChannel *ChanLogger = nil

type ChanLogger struct {
	logChan  chan log.LogStruct
	Handlers []Handler
	quit     chan string
}

type Handler interface {
	Handle(log log.LogStruct) error
}

func NewChanLogger(bufferSize int) *ChanLogger {
	logger := &ChanLogger{
		logChan:  make(chan log.LogStruct, bufferSize),
		quit:     make(chan string),
		Handlers: []Handler{},
	}

	go func() {
		for {
			select {
			case log := <-logger.logChan:
				for _, handler := range logger.Handlers {
					handler.Handle(log)
				}
			case <-logger.quit:
				return
			}
		}
	}()

	return logger
}

func (l *ChanLogger) Info(a ...any) {
	str := fmt.Sprintln(a...)

	l.logChan <- log.LogStruct{
		Level:   "INFO",
		Message: strings.TrimSpace(str),
		Time:    time.Now(),
	}
}

func (l *ChanLogger) Warn(a ...any) {
	str := fmt.Sprintln(a...)
	l.logChan <- log.LogStruct{
		Level:   "WARN",
		Message: strings.TrimSpace(str),
		Time:    time.Now(),
	}
}

func (l *ChanLogger) Error(msg error) {
	l.logChan <- log.LogStruct{
		Level:   "ERROR",
		Message: strings.TrimSpace(traceError(msg)),
		Time:    time.Now(),
	}
}

func traceError(err error) string {
	if err == nil {
		return ""
	}

	var lines []string
	prefix := ""

	for err != nil {
		// выводим только текущий уровень ошибки
		lines = append(lines, fmt.Sprintf("%s└── %v", prefix, err))
		prefix += "    " // добавляем отступ для следующего уровня
		err = errors.Unwrap(err)
	}

	return strings.Join(lines, "\n") + "\n"
}

func (l *ChanLogger) Debug(a ...any) {
	str := fmt.Sprintln(a...)
	l.logChan <- log.LogStruct{
		Level:   "DEBUG",
		Message: strings.TrimSpace(str),
		Time:    time.Now(),
	}
}

func (l *ChanLogger) AddHandler(h Handler) {
	l.Handlers = append(l.Handlers, h)
}

func (l *ChanLogger) Close() {
	l.quit <- "quit"
	close(l.quit)
	close(l.logChan)
}
