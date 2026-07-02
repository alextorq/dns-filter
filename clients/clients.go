// Package clients owns client persistence orchestration and the in-memory
// exclusion snapshot consumed by the DNS hot path.
package clients

import (
	"context"
	"net"
	"sync"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
	change_filter "github.com/alextorq/dns-filter/clients/use-cases/change-filter"
	"github.com/alextorq/dns-filter/clients/use-cases/create"
	"github.com/alextorq/dns-filter/clients/use-cases/remove"
	"github.com/alextorq/dns-filter/clients/use-cases/update"
)

// Repo is the persistence contract consumed by the clients feature. The
// concrete clients/db.Repo is constructed in main.
type Repo interface {
	GetAll() ([]db.Client, error)
	GetExcluded() ([]db.Client, error)
	GetByID(id uint) (*db.Client, error)
	Create(*db.Client) error
	UpdateFields(id uint, fields map[string]any) error
	Delete(id uint) error
}

// ExclusionStore is the mutable snapshot shared with the DNS server.
type ExclusionStore interface {
	Replace([]db.Client)
	AddClient(*db.Client)
	RemoveClient(*db.Client)
}

type Module struct {
	repo       Repo
	exclusions ExclusionStore
	// Mutations update persistent and in-memory state as one logical operation.
	// Serialize them with full snapshot rebuilds so a stale Sync result cannot
	// overwrite a concurrent create/remove/toggle.
	mutationMu sync.Mutex
}

func NewModule(repo Repo, exclusions ExclusionStore) *Module {
	return &Module{repo: repo, exclusions: exclusions}
}

// Sync rebuilds the in-memory exclusion snapshot from persistent state.
func (m *Module) Sync() error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	rows, err := m.repo.GetExcluded()
	if err != nil {
		return err
	}
	m.exclusions.Replace(rows)
	return nil
}

func (m *Module) List() ([]db.Client, error) {
	return m.repo.GetAll()
}

func (m *Module) Create(in create.Input) (*db.Client, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return create.Create(m.repo, m.exclusions, in)
}

func (m *Module) Update(in update.Input) (*db.Client, error) {
	return update.Update(m.repo, in)
}

func (m *Module) ChangeFilter(id uint, filtered bool) (*db.Client, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return change_filter.ChangeFilter(m.repo, m.exclusions, id, filtered)
}

func (m *Module) Remove(id uint) error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return remove.Remove(m.repo, m.exclusions, id)
}

// Discover runs a best-effort LAN scan and annotates results from the injected
// repository. A repository failure only removes the informational
// already_registered badge; discovery results remain useful.
func (m *Module) Discover(ctx context.Context, opts discovery.DiscoverOptions) (*discovery.Result, error) {
	res, err := discovery.Discover(ctx, opts)
	if err != nil {
		return nil, err
	}
	rows, err := m.repo.GetAll()
	if err == nil {
		annotateRegistered(res.Devices, rows)
	}
	return res, nil
}

func annotateRegistered(devices []discovery.Device, rows []db.Client) {
	knownIPs := make(map[string]struct{}, len(rows))
	knownMACs := make(map[string]struct{}, len(rows))
	for _, c := range rows {
		if c.IP != "" {
			knownIPs[c.IP] = struct{}{}
		}
		if c.MAC != "" {
			knownMACs[normalizeMAC(c.MAC)] = struct{}{}
		}
	}
	for i := range devices {
		if _, ok := knownIPs[devices[i].IP]; ok {
			devices[i].AlreadyRegistered = true
			continue
		}
		if devices[i].MAC != "" {
			_, devices[i].AlreadyRegistered = knownMACs[normalizeMAC(devices[i].MAC)]
		}
	}
}

func normalizeMAC(mac string) string {
	parsed, err := net.ParseMAC(mac)
	if err != nil {
		return mac
	}
	return parsed.String()
}
