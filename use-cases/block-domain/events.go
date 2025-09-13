package block_domain

import (
	"context"
	"time"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/events"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

var conf = config.GetConfig()

func SendEventAboutBlockDomain(domain string) error {
	if conf.UseInfluxdb {
		client := events.GetClient()
		org := conf.InfluxdbOrg
		bucket := conf.InfluxdbBucket
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
	return nil
}
