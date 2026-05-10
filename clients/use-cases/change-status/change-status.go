package change_status

import (
	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/clients/db"
)

// ChangeClientStatus persists the active flag for an exclude-client and keeps
// the in-memory dictionary in sync. Without the in-memory update the toggle is
// invisible to the DNS hot path until the next process restart (issue #27).
func ChangeClientStatus(id uint, isActive bool) error {
	cl, err := db.GetClientById(id)
	if err != nil {
		return err
	}
	if err := db.UpdateClientIsActive(id, isActive); err != nil {
		return err
	}
	dict := clients.GetClients()
	if isActive {
		dict.AddClient(cl.UserId)
	} else {
		dict.RemoveClient(cl.UserId)
	}
	return nil
}
