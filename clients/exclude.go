package clients

import (
	"github.com/alextorq/dns-filter/clients/use-cases/add"
	"github.com/alextorq/dns-filter/clients/use-cases/remove"
	"github.com/alextorq/dns-filter/clients/use-cases/update"
)

func UpdateClients() {
	update.UpdateClients()
}

func AddClient(id string) {
	add.AddClient(id)
}

func RemoveClient(id uint) {
	remove.RemoveClient(id)
}
