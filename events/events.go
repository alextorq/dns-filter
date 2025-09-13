package events

import (
	"github.com/alextorq/dns-filter/config"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
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
	url := conf.InfluxdbUrl
	client := influxdb2.NewClient(url, token)
	return client
}
