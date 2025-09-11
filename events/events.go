package events

import (
	"context"
	"time"

	"github.com/alextorq/dns-filter/config"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

var conf = config.GetConfig()

var client influxdb2.Client = nil

func GetClient() influxdb2.Client {
	if client != nil {
		return client
	}
	c := Create()
	client = c
	return client
}

func Create() influxdb2.Client {
	token := conf.InfluxdbToken
	url := "http://localhost:8086"
	client := influxdb2.NewClient(url, token)
	return client
}

func SendEventAboutBlockDomain(domain string) error {
	client := GetClient()
	org := "filter-dns"
	bucket := "events"
	writeAPI := client.WriteAPIBlocking(org, bucket)

	tags := map[string]string{
		"domain": domain,
	}
	fields := map[string]interface{}{
		"count": 1,
	}

	point := write.NewPoint("block-domain", tags, fields, time.Now())

	return writeAPI.WritePoint(context.Background(), point)
}
