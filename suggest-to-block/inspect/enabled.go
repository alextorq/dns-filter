package inspect

import "sync/atomic"

// EnabledState is the process-owned runtime flag for reputation enrichment.
// Construct it in the composition root and inject its methods into the settings
// Apply hook and both feature gates. Keeping the atomic on an instance prevents
// tests (and future multiple application instances) from sharing package state.
// The zero value is ready to use and disabled.
type EnabledState struct {
	enabled atomic.Bool
}

func NewEnabledState() *EnabledState { return &EnabledState{} }

// Set writes the new value. It is called:
//   - на старте: HydrateAll → Apply (effective = БД override → env default);
//   - в рантайме: PUT /api/settings/suggest_inspect_enabled → Apply.
func (s *EnabledState) Set(v bool) { s.enabled.Store(v) }

// Enabled is read lock-free by the worker and suggest collector gates.
func (s *EnabledState) Enabled() bool { return s.enabled.Load() }
