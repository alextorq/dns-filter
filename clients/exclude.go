package clients

import (
	"github.com/alextorq/dns-filter/clients/use-cases/add"
	change_status "github.com/alextorq/dns-filter/clients/use-cases/change-status"
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

func ChangeClientStatus(id uint, isActive bool) error {
	return change_status.ChangeClientStatus(id, isActive)
}
