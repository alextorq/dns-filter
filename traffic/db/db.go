package db

import "time"

// DomainTraffic is the unified per-device DNS query counter. One row aggregates
// how many times a single device (identified by ClientKind/ClientValue) queried
// a single Domain with a single verdict (Blocked) on a single Day. It is a
// counter table — no per-query rows — so the high-volume DNS path collapses into
// additive upserts (see Repo.UpsertBatch).
//
// The composite UNIQUE index (client_kind, client_value, blocked, domain, day)
// is the conflict target of the additive upsert: a repeated (device, domain,
// verdict, day) bumps Count instead of inserting a new row.
//
// Secondary indexes back the read queries that later steps add:
//   - (client_kind, client_value, day) — per-device dashboard rollups;
//   - (day)                            — the daily retention prune (DeleteOlderThan);
//   - (blocked, day)                   — legacy block-stats aggregation (SUM WHERE blocked).
//
// ClientValue is the stable device key (MAC when known, else IP). ClientIP is
// only the last IP the device was seen using — informational, for the UI to tell
// two same-vendor devices apart; it is NOT part of the unique key.
type DomainTraffic struct {
	ID uint `gorm:"primarykey" json:"id"`

	ClientKind  string `gorm:"type:varchar(8);not null;uniqueIndex:idx_traffic_key,priority:1;index:idx_traffic_device,priority:1" json:"client_kind"`
	ClientValue string `gorm:"type:varchar(64);not null;uniqueIndex:idx_traffic_key,priority:2;index:idx_traffic_device,priority:2" json:"client_value"`
	ClientIP    string `gorm:"type:varchar(64)" json:"client_ip"`
	Domain      string `gorm:"type:varchar(255);not null;uniqueIndex:idx_traffic_key,priority:4" json:"domain"`
	Blocked     bool   `gorm:"not null;uniqueIndex:idx_traffic_key,priority:3;index:idx_traffic_blocked_day,priority:1" json:"blocked"`

	Day      time.Time `gorm:"not null;uniqueIndex:idx_traffic_key,priority:5;index:idx_traffic_device,priority:3;index:idx_traffic_day;index:idx_traffic_blocked_day,priority:2" json:"day"`
	Count    int64     `gorm:"not null;default:0" json:"count"`
	LastSeen time.Time `json:"last_seen"`
}
