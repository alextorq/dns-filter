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
}

type ConsoleHandler struct{}

func (h *ConsoleHandler) Handle(log log.LogStruct) error {
	color, ok := colors[log.Level]
	if !ok {
		color = colors["RESET"]
	}

	fmt.Printf("%s[%s] [%s]%s %s\n",
		color,
		log.Time.Format("2006-01-02 15:04:05"), // timestamp
		log.Level,                              // уровень
		colors["RESET"],                        // сброс цвета
		log.Message,                            // текст сообщения
	)

	return nil
}
