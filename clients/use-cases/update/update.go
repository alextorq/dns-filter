package update

import (
	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/clients/db"
)

func UpdateClients() {
	activeClients, err := db.GetAllActiveClients()
	if err != nil {
		panic(err)
	}
	cl := make([]string, 0, len(activeClients))
	for _, client := range activeClients {
		cl = append(cl, client.UserId)
	}
	client := clients.GetClients()
	client.UpdateClients(cl)
}
