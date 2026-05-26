// Package db is the persistent store for hostnames learned about LAN devices.
//
// The only source today is the background mDNS sweep (clients/hostnames), which
// resolves each discovered IP to a MAC via the arpwatcher cache and records the
// MAC→hostname pair here. Rows are keyed by MAC on purpose: a phone's MAC is
// stable for the lifetime of its association with the network (even when it is
// a privacy-randomized MAC), whereas its IP rotates with DHCP. Keying by IP
// would let a hostname "stick" to an address that DHCP later hands to a
// different device — exactly the staleness MAC-keying avoids. Devices whose MAC
// is not (yet) known are simply skipped by the collector, not stored under an
// IP.
//
// The traffic dashboard joins this table on the device MAC to show a friendly
// name instead of the OUI vendor (which is "Locally administered" for the very
// randomized-MAC phones this feature targets).
package db

import (
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HostName is one MAC→hostname fact learned from the LAN. LastSeen drives
// retention: a device that stops announcing itself is pruned once its last
// sighting falls outside the retention window.
type HostName struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	MAC      string    `gorm:"uniqueIndex" json:"mac"`
	Hostname string    `json:"hostname"`
	LastSeen time.Time `json:"last_seen"`
}

// Repo is the DI adapter over the host_names table. Construct at the
// composition root and pass to the hostname collector and the traffic handler.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo { return &Repo{db: conn} }

// Upsert records (or refreshes) the hostname for a MAC. The MAC is normalized
// to the canonical lowercase colon form so it matches the keys traffic stores
// (net.HardwareAddr.String()). Empty mac or hostname are no-ops — there is
// nothing useful to key on or show. UpdatedAt/LastSeen are written explicitly
// because the ON CONFLICT update path does not run GORM's autoUpdateTime hook.
func (r *Repo) Upsert(mac, hostname string) error {
	mac = normalizeMAC(mac)
	hostname = strings.TrimSpace(hostname)
	if mac == "" || hostname == "" {
		return nil
	}
	now := time.Now()
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mac"}},
		DoUpdates: clause.AssignmentColumns([]string{"hostname", "last_seen", "updated_at"}),
	}).Create(&HostName{MAC: mac, Hostname: hostname, LastSeen: now, UpdatedAt: now}).Error
}

// AllAsMap returns every known MAC→hostname pair as a map keyed by the
// normalized MAC, ready for an in-memory join on the traffic read path. The
// table is LAN-sized (tens to low hundreds of rows), so loading it whole per
// dashboard request is cheaper than a per-device query.
func (r *Repo) AllAsMap() (map[string]string, error) {
	var rows []HostName
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[normalizeMAC(row.MAC)] = row.Hostname
	}
	return out, nil
}

// PruneOlderThan deletes rows last seen before now-window. Called after each
// sweep so departed devices don't accumulate. A non-positive window is treated
// as "no pruning" to avoid accidentally wiping the table on a misconfiguration.
func (r *Repo) PruneOlderThan(window time.Duration) error {
	if window <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-window)
	return r.db.Where("last_seen < ?", cutoff).Delete(&HostName{}).Error
}

// normalizeMAC canonicalizes a MAC to lowercase colon form via net.ParseMAC so
// that keys written by the collector match the values traffic records (both
// ultimately come from net.HardwareAddr.String()). Unparseable input falls back
// to a trimmed, lowercased copy rather than being dropped — a malformed key
// still round-trips consistently between write and read.
func normalizeMAC(mac string) string {
	mac = strings.TrimSpace(mac)
	if parsed, err := net.ParseMAC(mac); err == nil {
		return parsed.String()
	}
	return strings.ToLower(mac)
}
