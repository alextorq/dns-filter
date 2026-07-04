package inspect

import "testing"

func TestEnabledState_InstancesAreIndependent(t *testing.T) {
	first := NewEnabledState()
	second := NewEnabledState()

	first.Set(true)

	if !first.Enabled() {
		t.Fatal("first state should be enabled after Set(true)")
	}
	if second.Enabled() {
		t.Fatal("second state must not observe mutations from first state")
	}
}

func TestEnabledState_ZeroValueIsDisabled(t *testing.T) {
	var state EnabledState
	if state.Enabled() {
		t.Fatal("zero-value state must be disabled")
	}
}
