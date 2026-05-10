package change_status

import (
	"sync"

	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/clients/db"
)

// mu serializes ChangeClientStatus calls. The use case touches two pieces of
// state — the DB row and the in-memory exclusion dict — and we need them to
// agree at the end. Without the lock, two concurrent toggles can have their
// dict mutations interleave so the loser's verdict lands after the winner's,
// leaving DB and memory disagreeing until the next UpdateClients() pass.
var mu sync.Mutex

// ChangeClientStatus persists the active flag for an exclude-client and keeps
// the in-memory dictionary in sync. Without the in-memory update the toggle is
// invisible to the DNS hot path until the next process restart (issue #27).
func ChangeClientStatus(id uint, isActive bool) error {
	mu.Lock()
	defer mu.Unlock()

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
