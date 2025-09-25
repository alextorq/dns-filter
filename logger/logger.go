package logger

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alextorq/dns-filter/logger/log"
)

// LogLevel — минимальный уровень логирования
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

func (l LogLevel) String() string {
	return [...]string{"DEBUG", "INFO", "WARN", "ERROR"}[l]
}

// ChanLogger — основной логгер
type ChanLogger struct {
	logChan  chan log.LogStruct
	Handlers []Handler
	quit     chan struct{}
	level    LogLevel
}

// Handler — интерфейс для обработчиков
type Handler interface {
	Handle(log log.LogStruct) error
}

// NewChanLogger — создаем логгер с буфером и минимальным уровнем логирования
func NewChanLogger(bufferSize int, level string) *ChanLogger {
	logger := &ChanLogger{
		logChan:  make(chan log.LogStruct, bufferSize),
		quit:     make(chan struct{}),
		Handlers: []Handler{},
		level:    LogLevelFromString(level),
	}

	go logger.loop()
	return logger
}

// loop — основной цикл логгера
func (l *ChanLogger) loop() {
	for {
		select {
		case logStruct := <-l.logChan:
			if shouldLog(logStruct.Level, l.level) {
				for _, h := range l.Handlers {
					h.Handle(logStruct)
				}
			}
		case <-l.quit:
			return
		}
	}
}

// shouldLog — проверяем, подходит ли уровень
func shouldLog(msgLevel string, minLevel LogLevel) bool {
	lvl := LogLevelFromString(msgLevel)
	return lvl >= minLevel
}

// LogLevelFromString — сопоставление строки уровня с LogLevel
func LogLevelFromString(level string) LogLevel {
	l, _ := LogLevelFromStringOrError(level)
	return l
}

func LogLevelFromStringOrError(level string) (LogLevel, error) {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARN":
		return WARN, nil
	case "ERROR":
		return ERROR, nil
	default:
		return ERROR, fmt.Errorf("unknown log level: %s", level)
	}
}

// AddHandler — добавляем обработчик
func (l *ChanLogger) AddHandler(h Handler) {
	l.Handlers = append(l.Handlers, h)
}

// Info, Warn, Debug, Error — методы логирования
func (l *ChanLogger) Info(a ...any) {
	l.send("INFO", fmt.Sprintln(a...))
}

func (l *ChanLogger) Warn(a ...any) {
	l.send("WARN", fmt.Sprintln(a...))
}

func (l *ChanLogger) Debug(a ...any) {
	l.send("DEBUG", fmt.Sprintln(a...))
}

func (l *ChanLogger) Error(err error) {
	l.send("ERROR", traceError(err))
}

// send — отправка в канал
func (l *ChanLogger) send(level, msg string) {
	l.logChan <- log.LogStruct{
		Level:   level,
		Message: strings.TrimSpace(msg),
		Time:    time.Now(),
	}
}

// traceError — разворачиваем цепочку ошибок
func traceError(err error) string {
	if err == nil {
		return ""
	}
	var lines []string
	prefix := ""
	for err != nil {
		lines = append(lines, fmt.Sprintf("%s└── %v", prefix, err))
		prefix += "    "
		err = errors.Unwrap(err)
	}
	return strings.Join(lines, "\n") + "\n"
}

// Close — корректно закрываем логгер
func (l *ChanLogger) Close() {
	close(l.quit)
	close(l.logChan)
}

func (l *ChanLogger) UpdateLogLevel(level string) {
	l.level = LogLevelFromString(level)
}
