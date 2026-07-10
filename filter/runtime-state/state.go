// Package runtime_state owns the mutable, process-local filter toggle and
// pause deadline. Boot configuration stays immutable and separate in config.
package runtime_state

import "sync/atomic"

type State struct {
	enabled         atomic.Bool
	pausedUntilUnix atomic.Int64
}

func New(enabled bool) *State {
	s := &State{}
	s.enabled.Store(enabled)
	return s
}

func (s *State) Enabled() bool { return s.enabled.Load() }

func (s *State) SetEnabled(enabled bool) { s.enabled.Store(enabled) }

// ToggleAndResume flips Enabled without losing concurrent toggles and clears
// any pause so re-enabling can never remain hidden behind an old deadline.
func (s *State) ToggleAndResume() bool {
	for {
		old := s.enabled.Load()
		if s.enabled.CompareAndSwap(old, !old) {
			s.pausedUntilUnix.Store(0)
			return !old
		}
	}
}

func (s *State) PausedUntil() int64 { return s.pausedUntilUnix.Load() }

func (s *State) SetPausedUntil(until int64) { s.pausedUntilUnix.Store(until) }

// ClearPause returns true when an active/stored deadline was removed.
func (s *State) ClearPause() bool { return s.pausedUntilUnix.Swap(0) != 0 }
