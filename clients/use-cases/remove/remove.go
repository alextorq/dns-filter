package remove

import (
	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/clients/db"
)

func RemoveClient(id uint) error {
	clientById, err := db.GetClientById(id)
	if err != nil {
		return err
	}
	err = db.DeleteClient(id)
	if err != nil {
		return err
	}
	clients.GetClients().RemoveClient(clientById.UserId)
	return nil
}
