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
	var lines []string
	prefix := ""

	for err != nil {
		lines = append(lines, fmt.Sprintf("%s└── %s", prefix, err.Error()))
		prefix += "    " // добавляем отступ для следующего уровня
		err = errors.Unwrap(err)
	}

	return fmt.Sprintln(linesToString(lines))
}

func linesToString(lines []string) string {
	result := ""
	for _, l := range lines {
		result += l + "\n"
	}
	return result
}

func (l *ChanLogger) Debug(msg string) {
	l.logChan <- log.LogStruct{
		Level:   "DEBUG",
		Message: strings.TrimSpace(msg),
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
