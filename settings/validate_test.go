package settings

import "testing"

func TestValidatePositiveInt(t *testing.T) {
	if err := ValidatePositiveInt("1"); err != nil {
		t.Errorf("1 should be valid: %v", err)
	}
	if err := ValidatePositiveInt("0"); err == nil {
		t.Error("0 must be rejected")
	}
	if err := ValidatePositiveInt("-3"); err == nil {
		t.Error("negative must be rejected")
	}
	if err := ValidatePositiveInt("nope"); err == nil {
		t.Error("non-integer must be rejected")
	}
}

func TestValidateIntRange(t *testing.T) {
	v := ValidateIntRange(1, 3650)

	// Happy path: in-range values (lower bound, a typical value, upper bound).
	for _, ok := range []string{"1", "30", "3650", " 30 "} {
		if err := v(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}

	// Negative: below the lower bound, the upper bound + 1, and a non-integer.
	for _, bad := range []string{"0", "-1", "3651", "abc", ""} {
		if err := v(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}
