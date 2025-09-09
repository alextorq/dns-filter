package loki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/alextorq/dns-filter/logger/log"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// LokiEntry и Stream
type LokiEntry struct {
	Ts   string `json:"ts"`
	Line string `json:"line"`
}

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type LokiPayload struct {
	Streams []LokiStream `json:"streams"`
}

type LokiHandler struct {
	URL    string
	Labels string
	Attrs  []slog.Attr
	Group  []string
}

func (h *LokiHandler) Handle(log log.LogStruct) error {
	// Формируем stream-лейблы
	stream := map[string]string{
		"job": "news",
		"env": "local",
		// уровень лога тоже в Labels
		"level": log.Level,
	}

	// Добавляем Attrs
	for _, attr := range h.Attrs {
		stream[attr.Key] = fmt.Sprint(attr.Value.Any())
	}

	// Добавляем Group (если есть)
	if len(h.Group) > 0 {
		stream["_group"] = strings.Join(h.Group, ".")
	}

	// Время в наносекундах
	timestamp := strconv.FormatInt(log.Time.UnixNano(), 10)

	payload := LokiPayload{
		Streams: []LokiStream{
			{
				Stream: stream,
				Values: [][2]string{
					{timestamp, log.Message},
				},
			},
		},
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post(h.URL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("loki returned status %s", resp.Status)
	}

	return nil
}
