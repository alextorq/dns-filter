package console

import (
	"fmt"

	"github.com/alextorq/dns-filter/logger/log"
)

var colors = map[string]string{
	"INFO":  "\033[32m", // зелёный
	"WARN":  "\033[33m", // жёлтый
	"ERROR": "\033[31m", // красный
	"DEBUG": "\033[36m", // голубой
	"RESET": "\033[0m",  // сброс цвета
	"TIME":  "\033[34m", // синий для времени
}

type ConsoleHandler struct{}

func (h *ConsoleHandler) Handle(l log.LogStruct) error {
	levelColor, ok := colors[l.Level]
	if !ok {
		levelColor = colors["RESET"]
	}

	fmt.Printf("[%s%s%s] [%s%s%s] %s\n",
		colors["TIME"],
		l.Time.Format("2006-01-02 15:04:05"),
		colors["RESET"],                      // время синим
		levelColor, l.Level, colors["RESET"], // уровень из мапы
		l.Message, // сообщение как есть
	)

	return nil
}
