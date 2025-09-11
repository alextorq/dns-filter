package dns

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"

	"github.com/miekg/dns"
)

func MakeRequest(domain string) (*dns.Msg, error) {
	// DoH сервер (например, Cloudflare)
	url := "https://cloudflare-dns.com/dns-query"

	// создаём DNS-запрос
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)

	// сериализуем в бинарный wire формат
	data, _ := m.Pack()

	// HTTP клиент с TLS
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	// делаем POST (DoH поддерживает GET и POST, но POST надёжнее)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/dns-message")

	resp, err := client.Do(req)
	defer resp.Body.Close()

	if err != nil {
		return nil, err
	}

	body, _ := io.ReadAll(resp.Body)

	// разбираем ответ обратно в dns.Msg
	in := new(dns.Msg)
	err = in.Unpack(body)
	return in, err
}
