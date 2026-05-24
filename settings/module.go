// Package settings is a generic, typed key→value runtime-configuration store.
//
// It knows nothing about the concrete settings the app exposes: those are
// declared at the composition root (see the settings wiring in package main),
// where each Setting descriptor binds a key to its default, validation and an
// Apply hook that pushes the value into the running process (logger, DoH
// resolver, cache, …). This package only owns the mechanics: precedence
// (DB-override beats env default), validation-before-persist, persist-before-
// apply, and serialized writes.
//
// Effective value precedence is: DB row (if present) → env/compiled default.
// Persistence is the source of truth once a value is set; deleting the row
// (Reset) hands the setting back to env control.
package settings

import (
	"errors"
	"fmt"
	"sync"
)

// ErrUnknownKey is returned when a key is not in the registry. The web layer
// maps it to 404.
var ErrUnknownKey = errors.New("unknown setting key")

// ErrInvalidValue wraps a descriptor's validation failure. The web layer maps
// it to 400.
var ErrInvalidValue = errors.New("invalid setting value")

// Repo is the persistence port. *settings/db.Repo satisfies it structurally.
type Repo interface {
	Get(key string) (value string, found bool, err error)
	Set(key, value string) error
	Delete(key string) error
}

// Setting is one registry descriptor. Construct at the composition root.
//
// Default is the env/compiled fallback (already resolved from env by config),
// captured as a string. Validate rejects bad input before anything is
// persisted. Apply parses the validated raw value and pushes it into the
// runtime sink; it is the single path used both for runtime changes and for
// startup hydration.
type Setting struct {
	Key      string
	Type     string // UI hint: "enum" | "url" | "ip-list" | "bool" | "duration" | "int"
	Enum     []string
	Default  string
	Validate func(raw string) error
	Apply    func(raw string) error
}

// Effective is the resolved view of a setting for the API: its current value,
// where it came from, and the metadata a generic UI needs to render an editor.
type Effective struct {
	Key        string   `json:"key"`
	Value      string   `json:"value"`
	Default    string   `json:"default"`
	Overridden bool     `json:"overridden"`
	Type       string   `json:"type"`
	Enum       []string `json:"enum,omitempty"`
}

// Module is the wired-up settings store.
type Module struct {
	repo  Repo
	mu    sync.Mutex
	order []string
	byKey map[string]Setting
}

func NewModule(repo Repo) *Module {
	return &Module{repo: repo, byKey: make(map[string]Setting)}
}

// Register adds descriptors to the registry. Call once at startup before
// HydrateAll; not safe to call concurrently with Set/HydrateAll.
func (m *Module) Register(settings ...Setting) {
	for _, s := range settings {
		if _, dup := m.byKey[s.Key]; !dup {
			m.order = append(m.order, s.Key)
		}
		m.byKey[s.Key] = s
	}
}

// effectiveLocked resolves the raw value for key: DB override if present,
// otherwise the env/compiled default. Caller must hold m.mu.
func (m *Module) effectiveLocked(key string) (raw string, overridden bool, err error) {
	s := m.byKey[key]
	stored, found, err := m.repo.Get(key)
	if err != nil {
		return "", false, err
	}
	if found {
		return stored, true, nil
	}
	return s.Default, false, nil
}

// HydrateAll applies the effective value of every registered setting to its
// runtime sink. Runs once at startup, after Register and before the DNS server
// serves traffic. A stored value that fails validation (e.g. a setting whose
// allowed set changed across versions) is logged via the returned error and
// the env/compiled default is applied instead, so a single bad row cannot
// brick startup. The returned error is informational — main logs it but does
// not abort.
func (m *Module) HydrateAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for _, key := range m.order {
		s := m.byKey[key]
		raw, _, err := m.effectiveLocked(key)
		if err != nil {
			errs = append(errs, fmt.Errorf("hydrate %s: read: %w", key, err))
			continue
		}
		if s.Validate != nil {
			if verr := s.Validate(raw); verr != nil {
				errs = append(errs, fmt.Errorf("hydrate %s: stored value %q invalid (%v), falling back to default %q", key, raw, verr, s.Default))
				raw = s.Default
			}
		}
		if s.Apply != nil {
			if aerr := s.Apply(raw); aerr != nil {
				errs = append(errs, fmt.Errorf("hydrate %s: apply: %w", key, aerr))
			}
		}
	}
	return errors.Join(errs...)
}

// Set validates, persists, then applies a new value for key. Persist happens
// before apply so the DB is the source of truth: if the process restarts right
// after a successful Set, the value survives. Validation runs first so an
// invalid value is neither stored nor applied. Writes are serialized; hot-path
// reads happen in the sinks (atomics), not here.
func (m *Module) Set(key, raw string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.byKey[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if s.Validate != nil {
		if err := s.Validate(raw); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidValue, err)
		}
	}
	if err := m.repo.Set(key, raw); err != nil {
		return fmt.Errorf("persist %s: %w", key, err)
	}
	if s.Apply != nil {
		if err := s.Apply(raw); err != nil {
			return fmt.Errorf("apply %s: %w", key, err)
		}
	}
	return nil
}

// Reset removes the DB override for key and applies the env/compiled default,
// handing the setting back to env control.
func (m *Module) Reset(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.byKey[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if err := m.repo.Delete(key); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	if s.Apply != nil {
		if err := s.Apply(s.Default); err != nil {
			return fmt.Errorf("apply default %s: %w", key, err)
		}
	}
	return nil
}

// List returns the effective view of every registered setting, in
// registration order.
func (m *Module) List() ([]Effective, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]Effective, 0, len(m.order))
	for _, key := range m.order {
		eff, err := m.effectiveViewLocked(key)
		if err != nil {
			return nil, err
		}
		out = append(out, eff)
	}
	return out, nil
}

// Get returns the effective view of a single setting.
func (m *Module) Get(key string) (Effective, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.byKey[key]; !ok {
		return Effective{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	return m.effectiveViewLocked(key)
}

func (m *Module) effectiveViewLocked(key string) (Effective, error) {
	s := m.byKey[key]
	raw, overridden, err := m.effectiveLocked(key)
	if err != nil {
		return Effective{}, err
	}
	return Effective{
		Key:        key,
		Value:      raw,
		Default:    s.Default,
		Overridden: overridden,
		Type:       s.Type,
		Enum:       s.Enum,
	}, nil
}
