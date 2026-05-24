package filter

import (
	"fmt"
	"strconv"
	"time"

	"github.com/alextorq/dns-filter/config"
)

// Keys under which the filter's runtime toggle is persisted in the settings
// KV table. They are deliberately NOT registered in the generic settings
// registry: the filter has its own endpoints (status / change-status / pause /
// resume) with pause-duration validation, and exposing paused_until as a
// free-form setting would let a caller bypass that. They share the settings
// table only as a persistence mechanism.
const (
	StateKeyEnabled     = "filter_enabled"
	StateKeyPausedUntil = "filter_paused_until"
)

// StateStore is the persistence port for the filter toggle. *settings/db.Repo
// satisfies it structurally.
type StateStore interface {
	Get(key string) (value string, found bool, err error)
	Set(key, value string) error
}

// PersistHook returns a Module.SetStateSink callback that writes the
// enabled/paused state to store. Write errors are logged, not propagated — a
// failed persist must not break the user-facing toggle (the in-memory atomic
// is already updated); the worst case is the state not surviving a restart.
func PersistHook(store StateStore, log Logger) func(enabled bool, pausedUntil int64) {
	return func(enabled bool, pausedUntil int64) {
		if err := store.Set(StateKeyEnabled, strconv.FormatBool(enabled)); err != nil {
			log.Error(fmt.Errorf("persist filter enabled state: %w", err))
		}
		if err := store.Set(StateKeyPausedUntil, strconv.FormatInt(pausedUntil, 10)); err != nil {
			log.Error(fmt.Errorf("persist filter pause state: %w", err))
		}
	}
}

// RestoreState loads the persisted toggle into conf at startup. It must run
// before the DNS server serves traffic so a restart preserves a deliberately
// disabled/paused filter.
//
// Precedence matches the rest of settings: a stored row overrides the
// compiled default (Enabled=true). A missing row leaves conf untouched. An
// already-expired pause deadline is normalized to 0 (no pause). A malformed
// stored value is ignored (leaves the default) rather than failing startup.
func RestoreState(store StateStore, conf *config.Config) error {
	if raw, found, err := store.Get(StateKeyEnabled); err != nil {
		return fmt.Errorf("load filter enabled state: %w", err)
	} else if found {
		if b, perr := strconv.ParseBool(raw); perr == nil {
			conf.Enabled.Store(b)
		}
	}

	if raw, found, err := store.Get(StateKeyPausedUntil); err != nil {
		return fmt.Errorf("load filter pause state: %w", err)
	} else if found {
		if until, perr := strconv.ParseInt(raw, 10, 64); perr == nil {
			if until > time.Now().Unix() {
				conf.PausedUntilUnix.Store(until)
			} else {
				conf.PausedUntilUnix.Store(0)
			}
		}
	}

	return nil
}
