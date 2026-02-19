package add

import (
	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/clients/db"
)

func AddClient(ip string) error {
	client := clients.GetClients()
	client.AddClient(ip)
	return db.AddClient(ip)
}
