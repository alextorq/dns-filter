package runtime_state

import "testing"

func TestState_InstancesAreIndependent(t *testing.T) {
	first := New(true)
	second := New(true)

	first.SetEnabled(false)
	first.SetPausedUntil(123)

	if first.Enabled() {
		t.Fatal("first state must be disabled")
	}
	if !second.Enabled() {
		t.Fatal("mutating first state changed second state")
	}
	if got := second.PausedUntil(); got != 0 {
		t.Fatalf("second pause = %d, want independent zero value", got)
	}
}

func TestState_ToggleAndResumeClearsPause(t *testing.T) {
	state := New(true)
	state.SetPausedUntil(123)

	if enabled := state.ToggleAndResume(); enabled {
		t.Fatal("toggle from enabled must return disabled")
	}
	if got := state.PausedUntil(); got != 0 {
		t.Fatalf("pause after toggle = %d, want 0", got)
	}
}
