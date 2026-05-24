package db

import "time"

// Setting is one persisted runtime-configuration override, stored as a
// stringly-typed key→value pair.
//
// The table is intentionally generic (KV) so adding a new dynamic setting
// needs no schema migration. The typed schema — default, validation and how
// a value is applied to the running process — lives in the settings registry
// (settings/registry.go), not here. A value is always serialized to its
// string form before storage and parsed back by the owning descriptor.
//
// Absence of a row means "not overridden": the effective value falls back to
// the env/compiled default. Deleting a row (DELETE /api/settings/:key) is how
// an operator hands the setting back to env control.
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
