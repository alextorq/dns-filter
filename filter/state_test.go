package filter

import (
	"errors"
	"strconv"
	"testing"
	"time"

	runtime_state "github.com/alextorq/dns-filter/filter/runtime-state"
)

type fakeStore struct {
	data   map[string]string
	getErr error
	setErr error
}

func newFakeStore() *fakeStore { return &fakeStore{data: map[string]string{}} }

func (f *fakeStore) Get(key string) (string, bool, error) {
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeStore) Set(key, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.data[key] = value
	return nil
}

// nopLogger satisfies the filter Logger port without side effects.
type nopLogger struct{}

func (nopLogger) Info(...any)  {}
func (nopLogger) Debug(...any) {}
func (nopLogger) Error(error)  {}

var stateTestNow = time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

type moduleTestClock struct{ now time.Time }

func (c moduleTestClock) Now() time.Time { return c.now }

func TestPersistHook_WritesBothKeys(t *testing.T) {
	store := newFakeStore()
	hook := PersistHook(store, nopLogger{})

	hook(false, 1700000000)

	if store.data[StateKeyEnabled] != "false" {
		t.Errorf("enabled = %q, want false", store.data[StateKeyEnabled])
	}
	if store.data[StateKeyPausedUntil] != "1700000000" {
		t.Errorf("paused_until = %q, want 1700000000", store.data[StateKeyPausedUntil])
	}
}

func TestNewModule_RejectsMissingRuntimeState(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("missing runtime state must fail during construction")
		}
	}()
	NewModule(nil, nil, nil, nil, nil, nopLogger{})
}

func TestNewModule_RejectsMissingClock(t *testing.T) {
	var typedNil *moduleTestClock
	for name, clock := range map[string]Clock{
		"nil":       nil,
		"typed nil": typedNil,
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("missing clock must fail during construction")
				}
			}()
			NewModule(nil, nil, nil, runtime_state.New(true), clock, nopLogger{})
		})
	}
}

func TestRestoreState_RejectsMissingRuntimeState(t *testing.T) {
	if err := RestoreState(newFakeStore(), nil, stateTestNow); err == nil {
		t.Fatal("missing runtime state must return an error")
	}
}

// A persist failure must not panic or propagate — the in-memory toggle has
// already taken effect; we only lose durability.
func TestPersistHook_SwallowsStoreError(t *testing.T) {
	store := newFakeStore()
	store.setErr = errors.New("disk full")
	hook := PersistHook(store, nopLogger{})

	hook(true, 0) // must not panic
}

func TestRestoreState_DisabledSurvivesRestart(t *testing.T) {
	store := newFakeStore()
	store.data[StateKeyEnabled] = "false"

	state := runtime_state.New(true) // compiled default

	if err := RestoreState(store, state, stateTestNow); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if state.Enabled() {
		t.Error("a persisted disabled filter must stay disabled after restore")
	}
}

func TestRestoreState_MissingRowKeepsDefault(t *testing.T) {
	store := newFakeStore() // empty

	state := runtime_state.New(true)

	if err := RestoreState(store, state, stateTestNow); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !state.Enabled() {
		t.Error("missing row must leave the compiled default (enabled) intact")
	}
}

func TestRestoreState_ExpiredPauseNormalizedToZero(t *testing.T) {
	store := newFakeStore()
	store.data[StateKeyEnabled] = "true"
	store.data[StateKeyPausedUntil] = strconv.FormatInt(stateTestNow.Add(-time.Hour).Unix(), 10)

	state := runtime_state.New(true)

	if err := RestoreState(store, state, stateTestNow); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := state.PausedUntil(); got != 0 {
		t.Errorf("expired pause must restore as 0, got %d", got)
	}
}

func TestRestoreState_PauseAtNowNormalizedToZero(t *testing.T) {
	store := newFakeStore()
	store.data[StateKeyPausedUntil] = strconv.FormatInt(stateTestNow.Unix(), 10)
	state := runtime_state.New(true)

	if err := RestoreState(store, state, stateTestNow); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := state.PausedUntil(); got != 0 {
		t.Fatalf("pause at now must be expired, got %d", got)
	}
}

func TestRestoreState_FuturePauseRestored(t *testing.T) {
	store := newFakeStore()
	future := stateTestNow.Add(time.Hour).Unix()
	store.data[StateKeyPausedUntil] = strconv.FormatInt(future, 10)

	state := runtime_state.New(true)

	if err := RestoreState(store, state, stateTestNow); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := state.PausedUntil(); got != future {
		t.Errorf("future pause must be restored, got %d want %d", got, future)
	}
}

// A malformed stored value must not fail startup — fall back to the default.
func TestRestoreState_MalformedValueIgnored(t *testing.T) {
	store := newFakeStore()
	store.data[StateKeyEnabled] = "yes-please"

	state := runtime_state.New(true)

	if err := RestoreState(store, state, stateTestNow); err != nil {
		t.Fatalf("restore must not fail on malformed value: %v", err)
	}
	if !state.Enabled() {
		t.Error("malformed value must leave the default (enabled) intact")
	}
}

// A store read error must surface so main can decide how loud to be.
func TestRestoreState_ReadErrorSurfaces(t *testing.T) {
	store := newFakeStore()
	store.getErr = errors.New("db down")

	state := runtime_state.New(true)
	if err := RestoreState(store, state, stateTestNow); err == nil {
		t.Error("expected read error to surface")
	}
}

// End-to-end: persist from one state, restore into a fresh one — the toggle
// round-trips across a simulated restart.
func TestFilterState_RoundTripsAcrossRestart(t *testing.T) {
	store := newFakeStore()
	hook := PersistHook(store, nopLogger{})

	// "Running" process disables the filter.
	hook(false, 0)

	// "Restart": a brand-new runtime state that defaults to enabled.
	restarted := runtime_state.New(true)
	if err := RestoreState(store, restarted, stateTestNow); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restarted.Enabled() {
		t.Error("filter should come back disabled after restart")
	}
}
